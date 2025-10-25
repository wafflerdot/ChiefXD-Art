package main

import (
	"fmt"
	"log"
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
