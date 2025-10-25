package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// OwnerID is a constant fallback for the primary owner user ID.
// You can also set OWNER_ID environment variable to override this at runtime.
const OwnerID = "757284149793914900"

// Permission bit flags (subset used)
const (
	PermAdministrator = 1 << 3 // 0x00000008
	PermManageGuild   = 1 << 5 // 0x00000020
)

// PermStore keeps the list of allowed role IDs per guild, with optional JSON file persistence.
// Users can run restricted commands if:
// - They are the owner (OWNER_ID), or
// - They have Administrator/Manage Guild permission, or
// - They have at least one role that is in the allowed set for the guild.
type PermStore struct {
	mu         sync.RWMutex
	guildRoles map[string]map[string]struct{} // guildID -> set(roleID)
	filePath   string                         // optional JSON file for persistence
}

func NewPermStore() *PermStore {
	return &PermStore{guildRoles: make(map[string]map[string]struct{})}
}

var perms = NewPermStore()

// ConfigureFile sets the JSON file path used for persistence.
func (ps *PermStore) ConfigureFile(path string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.filePath = path
}

// AddRole adds a role to the allowed set for a guild and persists.
func (ps *PermStore) AddRole(guildID, roleID string) {
	ps.mu.Lock()
	set := ps.guildRoles[guildID]
	if set == nil {
		set = make(map[string]struct{})
		ps.guildRoles[guildID] = set
	}
	set[roleID] = struct{}{}
	path := ps.filePath
	ps.mu.Unlock()

	// Persist outside the lock
	if path != "" {
		if err := ps.SaveToFile(); err != nil {
			log.Println("permissions save (add) error:", err)
		}
	}
}

// RemoveRole removes a role from the allowed set for a guild and persists.
func (ps *PermStore) RemoveRole(guildID, roleID string) {
	ps.mu.Lock()
	if set := ps.guildRoles[guildID]; set != nil {
		delete(set, roleID)
		if len(set) == 0 {
			delete(ps.guildRoles, guildID)
		}
	}
	path := ps.filePath
	ps.mu.Unlock()

	// Persist outside the lock
	if path != "" {
		if err := ps.SaveToFile(); err != nil {
			log.Println("permissions save (remove) error:", err)
		}
	}
}

// ListRoles returns a copy of the allowed role IDs for a guild.
func (ps *PermStore) ListRoles(guildID string) []string {
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

// IsOwner returns true if the user is the configured owner.
func IsOwner(userID string) bool {
	if env := strings.TrimSpace(os.Getenv("OWNER_ID")); env != "" {
		return userID == env
	}
	return userID == OwnerID
}

// HasAdminContextPermission returns true if the interaction member has Administrator or Manage Guild.
func HasAdminContextPermission(i *discordgo.InteractionCreate) bool {
	if i.Member == nil {
		return false
	}
	permsVal := i.Member.Permissions // permission integer snapshot for this context
	return (permsVal&PermAdministrator) != 0 || (permsVal&PermManageGuild) != 0
}

// IsAllowedForRestricted checks whether the invoking user can access restricted commands in a guild.
func (ps *PermStore) IsAllowedForRestricted(i *discordgo.InteractionCreate) bool {
	// DMs: allow only owner
	if i.GuildID == "" {
		if i.Member != nil && i.Member.User != nil {
			return IsOwner(i.Member.User.ID)
		}
		return false
	}

	// Owner or admin in this context
	if i.Member != nil && i.Member.User != nil && IsOwner(i.Member.User.ID) {
		return true
	}
	if HasAdminContextPermission(i) {
		return true
	}

	// Role-based check
	if i.Member == nil {
		return false
	}
	userRoles := i.Member.Roles
	if len(userRoles) == 0 {
		return false
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()
	allowed := ps.guildRoles[i.GuildID]
	if len(allowed) == 0 {
		return false
	}
	for _, r := range userRoles {
		if _, ok := allowed[r]; ok {
			return true
		}
	}
	return false
}

// FormatRoleList turns role IDs into a human-readable list using guild roles.
func FormatRoleList(s *discordgo.Session, guildID string, roleIDs []string) string {
	if len(roleIDs) == 0 {
		return "(none configured)"
	}

	roles, err := s.GuildRoles(guildID)
	if err != nil {
		log.Println("GuildRoles error:", err)
		return strings.Join(roleIDs, ", ")
	}

	nameByID := make(map[string]string, len(roles))
	for _, r := range roles {
		nameByID[r.ID] = r.Name
	}

	names := make([]string, 0, len(roleIDs))
	for _, id := range roleIDs {
		if n, ok := nameByID[id]; ok {
			names = append(names, n+" ("+id+")")
		} else {
			names = append(names, id)
		}
	}
	return strings.Join(names, ", ")
}

// JSON persistence

type permJSON struct {
	GuildRoles map[string][]string `json:"guild_roles"`
}

// SaveToFile writes the current permissions to disk in JSON format with an atomic rename.
func (ps *PermStore) SaveToFile() error {
	ps.mu.RLock()
	if ps.filePath == "" {
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
	path := ps.filePath
	ps.mu.RUnlock()

	payload := permJSON{GuildRoles: data}
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

// LoadFromFile populates the permissions from a JSON file if it exists.
func (ps *PermStore) LoadFromFile() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.filePath == "" {
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
	return nil
}
