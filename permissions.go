package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// OwnerID is a constant fallback for the primary owner user ID
// OWNER_ID environment variable will override this at runtime
const OwnerID = "757284149793914900"

// Permission bit flags (subset used)
const (
	PermAdministrator = 1 << 3 // 0x00000008
	PermManageGuild   = 1 << 5 // 0x00000020
)

// Supported DB dialects
const (
	DialectPostgres = "postgres"
	DialectMySQL    = "mysql"
)

// PermStore keeps the list of allowed role IDs per guild
// and a global user whitelist for DM usage.
// Backing storage:
// - If db is configured, data is stored in SQL tables
// - Otherwise falls back to JSON file persistence
type PermStore struct {
	mu         sync.RWMutex
	guildRoles map[string]map[string]struct{} // guildID -> set(roleID) (used for JSON fallback)
	userWL     map[string]struct{}            // set(userID) for JSON fallback
	filePath   string                         // JSON file path (fallback)

	db      *sql.DB
	dialect string
}

func NewPermStore() *PermStore {
	return &PermStore{guildRoles: make(map[string]map[string]struct{}), userWL: make(map[string]struct{})}
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

// ConfigureFile sets the JSON file path used for persistence (fallback mode)
func (ps *PermStore) ConfigureFile(path string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.filePath = path
}

// AddRole adds a role to the allowed set for a guild and persists
func (ps *PermStore) AddRole(guildID, roleID string) {
	// DB-backed path
	if ps.db != nil {
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
		return
	}

	// JSON fallback
	ps.mu.Lock()
	set := ps.guildRoles[guildID]
	if set == nil {
		set = make(map[string]struct{})
		ps.guildRoles[guildID] = set
	}
	set[roleID] = struct{}{}
	path := ps.filePath
	ps.mu.Unlock()
	if path != "" {
		if err := ps.SaveToFile(); err != nil {
			log.Println("permissions save (add) error:", err)
		}
	}
}

// RemoveRole removes a role from the allowed set for a guild and persists
func (ps *PermStore) RemoveRole(guildID, roleID string) {
	// DB-backed path
	if ps.db != nil {
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
		return
	}

	// JSON fallback
	ps.mu.Lock()
	if set := ps.guildRoles[guildID]; set != nil {
		delete(set, roleID)
		if len(set) == 0 {
			delete(ps.guildRoles, guildID)
		}
	}
	path := ps.filePath
	ps.mu.Unlock()
	if path != "" {
		if err := ps.SaveToFile(); err != nil {
			log.Println("permissions save (remove) error:", err)
		}
	}
}

// ListRoles returns a copy of the allowed role IDs for a guild
func (ps *PermStore) ListRoles(guildID string) []string {
	// DB-backed path
	if ps.db != nil {
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
		return out
	}

	// JSON fallback
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	set := ps.guildRoles[guildID]
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// AddWhitelistedUser adds a user ID to the global DM whitelist
func (ps *PermStore) AddWhitelistedUser(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	if ps.db != nil {
		var stmt string
		switch ps.dialect {
		case DialectPostgres:
			stmt = `INSERT INTO whitelist_users (user_id) VALUES ($1) ON CONFLICT DO NOTHING`
		case DialectMySQL:
			stmt = `INSERT IGNORE INTO whitelist_users (user_id) VALUES (?)`
		}
		if _, err := ps.db.Exec(stmt, userID); err != nil {
			log.Println("whitelist insert error:", err)
		}
		return
	}
	ps.mu.Lock()
	ps.userWL[userID] = struct{}{}
	path := ps.filePath
	ps.mu.Unlock()
	if path != "" {
		if err := ps.SaveToFile(); err != nil {
			log.Println("whitelist save (add) error:", err)
		}
	}
}

// RemoveWhitelistedUser removes a user ID from the global DM whitelist
func (ps *PermStore) RemoveWhitelistedUser(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	if ps.db != nil {
		var stmt string
		switch ps.dialect {
		case DialectPostgres:
			stmt = `DELETE FROM whitelist_users WHERE user_id = $1`
		case DialectMySQL:
			stmt = `DELETE FROM whitelist_users WHERE user_id = ?`
		}
		if _, err := ps.db.Exec(stmt, userID); err != nil {
			log.Println("whitelist delete error:", err)
		}
		return
	}
	ps.mu.Lock()
	delete(ps.userWL, userID)
	path := ps.filePath
	ps.mu.Unlock()
	if path != "" {
		if err := ps.SaveToFile(); err != nil {
			log.Println("whitelist save (remove) error:", err)
		}
	}
}

// ListWhitelistedUsers returns a slice of whitelisted user IDs
func (ps *PermStore) ListWhitelistedUsers() []string {
	if ps.db != nil {
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
			}
		}
		return out
	}
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	out := make([]string, 0, len(ps.userWL))
	for id := range ps.userWL {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// IsUserWhitelisted checks if the userID is in the global DM whitelist
func (ps *PermStore) IsUserWhitelisted(userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	if ps.db != nil {
		var exists bool
		// portable exists query
		row := ps.db.QueryRow(`SELECT 1 FROM whitelist_users WHERE user_id = `+ps.param(1)+` LIMIT 1`, userID)
		var one int
		if err := row.Scan(&one); err != nil {
			return false
		}
		exists = one == 1
		return exists
	}
	ps.mu.RLock()
	_, ok := ps.userWL[userID]
	ps.mu.RUnlock()
	return ok
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

	// DMs: allow owner or whitelisted users
	if i.GuildID == "" {
		return IsOwner(userID) || ps.IsUserWhitelisted(userID)
	}

	// Owner or admin in this context
	if IsOwner(userID) {
		return true
	}
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

	allowed := ps.ListRoles(i.GuildID) // uses DB if configured
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

// JSON persistence (fallback)

type permJSON struct {
	GuildRoles    map[string][]string `json:"guild_roles"`
	UserWhitelist []string            `json:"user_whitelist"`
}

// SaveToFile writes the current permissions to disk in JSON format with an atomic rename
func (ps *PermStore) SaveToFile() error {
	ps.mu.RLock()
	if ps.filePath == "" || ps.db != nil {
		ps.mu.RUnlock()
		return nil
	}

	data := make(map[string][]string, len(ps.guildRoles))
	for g, set := range ps.guildRoles {
		list := make([]string, 0, len(set))
		for r := range set {
			list = append(list, r)
		}
		sort.Strings(list)
		data[g] = list
	}
	wl := make([]string, 0, len(ps.userWL))
	for u := range ps.userWL {
		wl = append(wl, u)
	}
	sort.Strings(wl)
	path := ps.filePath
	ps.mu.RUnlock()

	payload := permJSON{GuildRoles: data, UserWhitelist: wl}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadFromFile populates the permissions from a JSON file if it exists
func (ps *PermStore) LoadFromFile() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.filePath == "" || ps.db != nil {
		return nil
	}

	b, err := os.ReadFile(ps.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var payload permJSON
	if err := json.Unmarshal(b, &payload); err != nil {
		return err
	}

	ps.guildRoles = make(map[string]map[string]struct{}, len(payload.GuildRoles))
	for g, list := range payload.GuildRoles {
		set := make(map[string]struct{}, len(list))
		for _, r := range list {
			if strings.TrimSpace(r) == "" {
				continue
			}
			set[r] = struct{}{}
		}
		if len(set) > 0 {
			ps.guildRoles[g] = set
		}
	}
	ps.userWL = make(map[string]struct{}, len(payload.UserWhitelist))
	for _, u := range payload.UserWhitelist {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		ps.userWL[u] = struct{}{}
	}
	return nil
}

// Helper: dialect param placeholders
func (ps *PermStore) param(idx int) string {
	if ps.dialect == DialectPostgres {
		return fmt.Sprintf("$%d", idx)
	}
	return "?"
}
