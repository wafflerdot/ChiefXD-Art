package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// OwnerID is a constant fallback for the primary owner user ID
// OWNER_ID environment variable will override this at runtime
const OwnerID = "757284149793914900"

// Permission bit flags (subset used)
const (
	PermAdministrator = 1 << 3  // 0x00000008
	PermManageGuild   = 1 << 5  // 0x00000020
	PermManageRoles   = 1 << 28 // 0x10000000
)

// Supported DB dialects
const (
	DialectPostgres = "postgres"
	DialectMySQL    = "mysql"
)

// PermStore keeps the list of allowed role IDs per guild
// and a global user whitelist for DM usage.
// Backing storage: SQL database (PostgreSQL or MySQL)
// Includes in-memory caching for fast permission checks
type PermStore struct {
	db      *sql.DB
	dialect string

	// In-memory caches to avoid DB round-trips on every command
	rolesCache     sync.Map // guildID -> []string (role IDs)
	whitelistCache sync.Map // userID -> bool
	whitelistOnce  sync.Once
}

func NewPermStore() *PermStore {
	return &PermStore{}
}

var perms = NewPermStore()

// ConfigureDB connects to the database and ensures the permissions tables exist
// dialect: "postgres" or "mysql"
func (ps *PermStore) ConfigureDB(dialect, dsn string) error {
	db, err := sql.Open(dialect, dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping db: %w", err)
	}

	// Configure connection pool for better performance
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	ps.db = db
	ps.dialect = dialect

	// Create table if not exists (allowed roles per guild)
	var ddl string
	switch dialect {
	case DialectPostgres:
		ddl = `CREATE TABLE IF NOT EXISTS permissions (
			guild_id TEXT NOT NULL,
			role_id  TEXT NOT NULL,
			PRIMARY KEY (guild_id, role_id)
		)`
	case DialectMySQL:
		ddl = `CREATE TABLE IF NOT EXISTS permissions (
			guild_id VARCHAR(64) NOT NULL,
			role_id  VARCHAR(64) NOT NULL,
			PRIMARY KEY (guild_id, role_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	default:
		return fmt.Errorf("unsupported dialect: %s", dialect)
	}
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create permissions table: %w", err)
	}

	// Create global user whitelist table
	switch dialect {
	case DialectPostgres:
		ddl = `CREATE TABLE IF NOT EXISTS whitelist_users (
			user_id TEXT PRIMARY KEY
		)`
	case DialectMySQL:
		ddl = `CREATE TABLE IF NOT EXISTS whitelist_users (
			user_id VARCHAR(64) PRIMARY KEY
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	}
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create whitelist_users table: %w", err)
	}

	log.Printf("permissions: using %s database storage", dialect)
	return nil
}

// AddRole adds a role to the allowed set for a guild and persists
func (ps *PermStore) AddRole(guildID, roleID string) {
	if ps.db == nil {
		log.Println("permissions: database not configured")
		return
	}

	var sqlStmt string
	switch ps.dialect {
	case DialectPostgres:
		sqlStmt = `INSERT INTO permissions (guild_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	case DialectMySQL:
		sqlStmt = `INSERT IGNORE INTO permissions (guild_id, role_id) VALUES (?, ?)`
	}
	if _, err := ps.db.Exec(sqlStmt, guildID, roleID); err != nil {
		log.Println("permissions db insert error:", err)
	}
	// Invalidate cache for this guild
	ps.rolesCache.Delete(guildID)
}

// RemoveRole removes a role from the allowed set for a guild and persists
func (ps *PermStore) RemoveRole(guildID, roleID string) {
	if ps.db == nil {
		log.Println("permissions: database not configured")
		return
	}

	var sqlStmt string
	switch ps.dialect {
	case DialectPostgres:
		sqlStmt = `DELETE FROM permissions WHERE guild_id = $1 AND role_id = $2`
	case DialectMySQL:
		sqlStmt = `DELETE FROM permissions WHERE guild_id = ? AND role_id = ?`
	}
	if _, err := ps.db.Exec(sqlStmt, guildID, roleID); err != nil {
		log.Println("permissions db delete error:", err)
	}
	// Invalidate cache for this guild
	ps.rolesCache.Delete(guildID)
}

// ListRoles returns a copy of the allowed role IDs for a guild
func (ps *PermStore) ListRoles(guildID string) []string {
	// Check cache first
	if cached, ok := ps.rolesCache.Load(guildID); ok {
		return cached.([]string)
	}

	if ps.db == nil {
		log.Println("permissions: database not configured")
		return nil
	}

	var (
		rows *sql.Rows
		err  error
	)
	switch ps.dialect {
	case DialectPostgres:
		rows, err = ps.db.Query(`SELECT role_id FROM permissions WHERE guild_id = $1`, guildID)
	case DialectMySQL:
		rows, err = ps.db.Query(`SELECT role_id FROM permissions WHERE guild_id = ?`, guildID)
	}
	if err != nil {
		log.Println("permissions db list error:", err)
		return nil
	}
	defer rows.Close()
	out := make([]string, 0, 8)
	for rows.Next() {
		var roleID string
		if err := rows.Scan(&roleID); err != nil {
			log.Println("permissions db scan error:", err)
			continue
		}
		out = append(out, roleID)
	}

	// Store in cache
	ps.rolesCache.Store(guildID, out)
	return out
}

// AddWhitelistedUser adds a user ID to the global DM whitelist
func (ps *PermStore) AddWhitelistedUser(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	if ps.db == nil {
		log.Println("permissions: database not configured")
		return
	}

	var stmt string
	switch ps.dialect {
	case DialectPostgres:
		stmt = `INSERT INTO whitelist_users (user_id) VALUES ($1) ON CONFLICT DO NOTHING`
	case DialectMySQL:
		stmt = `INSERT IGNORE INTO whitelist_users (user_id) VALUES (?)`
	}
	if _, err := ps.db.Exec(stmt, userID); err != nil {
		log.Println("whitelist insert error:", err)
		return
	}
	// Update cache
	ps.whitelistCache.Store(userID, true)
}

// RemoveWhitelistedUser removes a user ID from the global DM whitelist
func (ps *PermStore) RemoveWhitelistedUser(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	if ps.db == nil {
		log.Println("permissions: database not configured")
		return
	}

	var stmt string
	switch ps.dialect {
	case DialectPostgres:
		stmt = `DELETE FROM whitelist_users WHERE user_id = $1`
	case DialectMySQL:
		stmt = `DELETE FROM whitelist_users WHERE user_id = ?`
	}
	if _, err := ps.db.Exec(stmt, userID); err != nil {
		log.Println("whitelist delete error:", err)
		return
	}
	// Update cache
	ps.whitelistCache.Store(userID, false)
}

// ListWhitelistedUsers returns a slice of whitelisted user IDs
func (ps *PermStore) ListWhitelistedUsers() []string {
	if ps.db == nil {
		log.Println("permissions: database not configured")
		return nil
	}

	rows, err := ps.db.Query(`SELECT user_id FROM whitelist_users`)
	if err != nil {
		log.Println("whitelist list error:", err)
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
			// Also warm the cache
			ps.whitelistCache.Store(id, true)
		}
	}
	return out
}

// WarmWhitelistCache pre-loads the whitelist cache from the database
// Call this once at startup for optimal first-request latency
func (ps *PermStore) WarmWhitelistCache() {
	ps.whitelistOnce.Do(func() {
		users := ps.ListWhitelistedUsers()
		log.Printf("permissions: warmed whitelist cache with %d users", len(users))
	})
}

// IsUserWhitelisted checks if the userID is in the global DM whitelist
func (ps *PermStore) IsUserWhitelisted(userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	if dbLessMode {
		return false
	}

	// Check cache first
	if cached, ok := ps.whitelistCache.Load(userID); ok {
		return cached.(bool)
	}

	if ps.db == nil {
		return false
	}

	// Query DB and cache result
	row := ps.db.QueryRow(`SELECT 1 FROM whitelist_users WHERE user_id = `+ps.param(1)+` LIMIT 1`, userID)
	var one int
	if err := row.Scan(&one); err != nil {
		ps.whitelistCache.Store(userID, false)
		return false
	}
	ps.whitelistCache.Store(userID, one == 1)
	return one == 1
}

// IsOwner returns true if the user is the configured owner
func IsOwner(userID string) bool {
	if env := strings.TrimSpace(os.Getenv("OWNER_ID")); env != "" {
		return userID == env
	}
	return userID == OwnerID
}

// invokingUserID safely retrieves the user ID from an interaction (works in guilds and DMs)
func invokingUserID(i *discordgo.InteractionCreate) string {
	// In guilds, user is in i.Member.User
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	// In DMs, user is directly in i.User
	if i.User != nil {
		return i.User.ID
	}
	// Fallback: check the underlying interaction
	if i.Interaction != nil && i.Interaction.User != nil {
		return i.Interaction.User.ID
	}
	return ""
}

// HasAdminContextPermission returns true if the interaction member has Administrator or Manage Guild permissions
func HasAdminContextPermission(i *discordgo.InteractionCreate) bool {
	if i.Member == nil {
		return false
	}
	permsVal := i.Member.Permissions // permission integer snapshot for this context
	return (permsVal&PermAdministrator) != 0 || (permsVal&PermManageGuild) != 0
}

// IsAllowedForRestricted checks whether the invoking user can access restricted commands in a guild or DM
func (ps *PermStore) IsAllowedForRestricted(i *discordgo.InteractionCreate) bool {
	userID := invokingUserID(i)

	// DMs: allow owner only in DB-less mode, otherwise owner or whitelisted users
	if i.GuildID == "" {
		if dbLessMode {
			return IsOwner(userID)
		}
		return IsOwner(userID) || ps.IsUserWhitelisted(userID)
	}

	// Owner override
	if IsOwner(userID) {
		return true
	}

	// DB-less mode uses Discord permissions only (Manage Roles or Administrator)
	if dbLessMode {
		if i.Member == nil {
			return false
		}
		permsVal := i.Member.Permissions
		return (permsVal&PermManageRoles) != 0 || (permsVal&PermAdministrator) != 0
	}

	// Admin in this context
	if HasAdminContextPermission(i) {
		return true
	}

	// Role-based check in guilds
	if i.Member == nil {
		return false
	}
	userRoles := i.Member.Roles
	if len(userRoles) == 0 {
		return false
	}

	allowed := ps.ListRoles(i.GuildID)
	if len(allowed) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	for _, r := range userRoles {
		if _, ok := set[r]; ok {
			return true
		}
	}
	return false
}

// FormatRoleList turns role IDs into a human-readable list as Discord mentions
// Example: <@&123>, <@&456>
func FormatRoleList(_ *discordgo.Session, _ string, roleIDs []string) string {
	if len(roleIDs) == 0 {
		return "(none configured)"
	}
	mentions := make([]string, 0, len(roleIDs))
	for _, id := range roleIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		mentions = append(mentions, "<@&"+id+">")
	}
	if len(mentions) == 0 {
		return "(none configured)"
	}
	return strings.Join(mentions, ", ")
}

// Helper: dialect param placeholders
func (ps *PermStore) param(idx int) string {
	if ps.dialect == DialectPostgres {
		return fmt.Sprintf("$%d", idx)
	}
	return "?"
}
