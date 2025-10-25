package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// ThresholdsStore persists active thresholds if a DB is configured.
// If no DB is configured, it is a no-op and values remain in-memory.
type ThresholdsStore struct{}

var thresholdsStore = &ThresholdsStore{}

// Init creates the thresholds table if DB is available and loads current values.
func (ts *ThresholdsStore) Init(ps *PermStore) error {
	if ps == nil || ps.db == nil {
		return nil
	}
	ddl := ""
	switch ps.dialect {
	case DialectPostgres:
		ddl = `CREATE TABLE IF NOT EXISTS thresholds (
			name  TEXT PRIMARY KEY,
			value DOUBLE PRECISION NOT NULL
		)`
	case DialectMySQL:
		ddl = `CREATE TABLE IF NOT EXISTS thresholds (
			name  VARCHAR(64) PRIMARY KEY,
			value DOUBLE NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	default:
		return fmt.Errorf("unsupported dialect: %s", ps.dialect)
	}
	if _, err := ps.db.Exec(ddl); err != nil {
		return fmt.Errorf("create thresholds table: %w", err)
	}

	// Create history table for audit logs
	switch ps.dialect {
	case DialectPostgres:
		ddl = `CREATE TABLE IF NOT EXISTS thresholds_history (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			old_value DOUBLE PRECISION,
			new_value DOUBLE PRECISION NOT NULL,
			user_id TEXT,
			guild_id TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`
	case DialectMySQL:
		ddl = `CREATE TABLE IF NOT EXISTS thresholds_history (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(64) NOT NULL,
			old_value DOUBLE NULL,
			new_value DOUBLE NOT NULL,
			user_id VARCHAR(64) NULL,
			guild_id VARCHAR(64) NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	}
	if _, err := ps.db.Exec(ddl); err != nil {
		return fmt.Errorf("create thresholds_history table: %w", err)
	}

	return ts.Load(ps)
}

// Load reads thresholds from DB and applies them to active globals.
func (ts *ThresholdsStore) Load(ps *PermStore) error {
	if ps == nil || ps.db == nil {
		return nil
	}
	rows, err := ps.db.Query(`SELECT name, value FROM thresholds`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var value float64
		if err := rows.Scan(&name, &value); err != nil {
			log.Println("thresholds scan:", err)
			continue
		}
		switch name {
		case "NuditySuggestive":
			NuditySuggestiveThreshold = value
		case "NudityExplicit":
			NudityExplicitThreshold = value
		case "Offensive":
			OffensiveThreshold = value
		case "AIGenerated":
			AIGeneratedThreshold = value
		}
	}
	return nil
}

// Set updates a single threshold in DB (and memory). value must be between 0 and 1.
func (ts *ThresholdsStore) Set(ps *PermStore, name string, value float64) error {
	// update memory
	switch name {
	case "NuditySuggestive":
		NuditySuggestiveThreshold = value
	case "NudityExplicit":
		NudityExplicitThreshold = value
	case "Offensive":
		OffensiveThreshold = value
	case "AIGenerated":
		AIGeneratedThreshold = value
	default:
		return fmt.Errorf("unknown threshold: %s", name)
	}
	if ps == nil || ps.db == nil {
		return nil
	}
	var stmt string
	switch ps.dialect {
	case DialectPostgres:
		stmt = `INSERT INTO thresholds (name, value) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`
	case DialectMySQL:
		stmt = `INSERT INTO thresholds (name, value) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE value = VALUES(value)`
	}
	_, err := ps.db.Exec(stmt, name, value)
	return err
}

// ResetOne resets a single threshold to default and persists.
func (ts *ThresholdsStore) ResetOne(ps *PermStore, name string) error {
	var def float64
	switch name {
	case "NuditySuggestive":
		def = DefaultNuditySuggestiveThreshold
	case "NudityExplicit":
		def = DefaultNudityExplicitThreshold
	case "Offensive":
		def = DefaultOffensiveThreshold
	case "AIGenerated":
		def = DefaultAIGeneratedThreshold
	default:
		return fmt.Errorf("unknown threshold: %s", name)
	}
	return ts.Set(ps, name, def)
}

// ResetAll resets all thresholds to their default values and persists if DB.
func (ts *ThresholdsStore) ResetAll(ps *PermStore) error {
	if err := ts.Set(ps, "NuditySuggestive", DefaultNuditySuggestiveThreshold); err != nil {
		return err
	}
	if err := ts.Set(ps, "NudityExplicit", DefaultNudityExplicitThreshold); err != nil {
		return err
	}
	if err := ts.Set(ps, "Offensive", DefaultOffensiveThreshold); err != nil {
		return err
	}
	if err := ts.Set(ps, "AIGenerated", DefaultAIGeneratedThreshold); err != nil {
		return err
	}
	return nil
}

// ThresholdChange represents an audit log entry for a set/reset operation.
type ThresholdChange struct {
	Name     string
	OldValue sql.NullFloat64
	NewValue float64
	UserID   sql.NullString
	GuildID  sql.NullString
	Created  time.Time
}

// LogChange writes an audit record; no-op when DB is not configured.
func (ts *ThresholdsStore) LogChange(ps *PermStore, name string, oldVal, newVal float64, userID, guildID string) error {
	if ps == nil || ps.db == nil {
		return nil
	}
	var stmt string
	switch ps.dialect {
	case DialectPostgres:
		stmt = `INSERT INTO thresholds_history (name, old_value, new_value, user_id, guild_id) VALUES ($1, $2, $3, $4, $5)`
	case DialectMySQL:
		stmt = `INSERT INTO thresholds_history (name, old_value, new_value, user_id, guild_id) VALUES (?, ?, ?, ?, ?)`
	}
	_, err := ps.db.Exec(stmt, name, oldVal, newVal, userID, guildID)
	return err
}

// History returns last N threshold changes ordered by newest first; empty when DB not configured.
func (ts *ThresholdsStore) History(ps *PermStore, limit int) ([]ThresholdChange, error) {
	changes := []ThresholdChange{}
	if ps == nil || ps.db == nil {
		return changes, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	var (
		rows *sql.Rows
		err  error
	)
	switch ps.dialect {
	case DialectPostgres:
		rows, err = ps.db.Query(`SELECT name, old_value, new_value, user_id, guild_id, created_at FROM thresholds_history ORDER BY created_at DESC LIMIT $1`, limit)
	case DialectMySQL:
		rows, err = ps.db.Query(`SELECT name, old_value, new_value, user_id, guild_id, created_at FROM thresholds_history ORDER BY created_at DESC LIMIT ?`, limit)
	}
	if err != nil {
		return changes, err
	}
	defer rows.Close()
	for rows.Next() {
		var c ThresholdChange
		if err := rows.Scan(&c.Name, &c.OldValue, &c.NewValue, &c.UserID, &c.GuildID, &c.Created); err != nil {
			log.Println("thresholds history scan:", err)
			continue
		}
		changes = append(changes, c)
	}
	return changes, nil
}

// HistoryFiltered returns last N changes for a specific threshold name.
func (ts *ThresholdsStore) HistoryFiltered(ps *PermStore, name string, limit int) ([]ThresholdChange, error) {
	changes := []ThresholdChange{}
	if ps == nil || ps.db == nil {
		return changes, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	var (
		rows *sql.Rows
		err  error
	)
	switch ps.dialect {
	case DialectPostgres:
		rows, err = ps.db.Query(`SELECT name, old_value, new_value, user_id, guild_id, created_at
			FROM thresholds_history WHERE name = $1 ORDER BY created_at DESC LIMIT $2`, name, limit)
	case DialectMySQL:
		rows, err = ps.db.Query(`SELECT name, old_value, new_value, user_id, guild_id, created_at
			FROM thresholds_history WHERE name = ? ORDER BY created_at DESC LIMIT ?`, name, limit)
	}
	if err != nil {
		return changes, err
	}
	defer rows.Close()
	for rows.Next() {
		var c ThresholdChange
		if err := rows.Scan(&c.Name, &c.OldValue, &c.NewValue, &c.UserID, &c.GuildID, &c.Created); err != nil {
			log.Println("thresholds history scan:", err)
			continue
		}
		changes = append(changes, c)
	}
	return changes, nil
}

// currentThresholdValue returns the active in-memory value by canonical name.
func currentThresholdValue(name string) float64 {
	switch name {
	case "NuditySuggestive":
		return NuditySuggestiveThreshold
	case "NudityExplicit":
		return NudityExplicitThreshold
	case "Offensive":
		return OffensiveThreshold
	case "AIGenerated":
		return AIGeneratedThreshold
	default:
		return 0
	}
}
