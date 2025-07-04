// Package autosqlite provides automatic SQLite database creation and migration
// from a schema string. It can create a new database, migrate an existing one
// to a new schema, and ensures data is preserved for common columns and tables.
//
// Features:
//   - Automatic schema migration
//   - Data preservation for common columns
//   - Backup and atomic replacement
//   - Skips migration if schema is unchanged
//   - Prevents backward migrations by tracking hashes of schemas already applied
//
// Usage:
//
//	package main
//
//	import (
//	    "log"
//	    "github.com/jes/autosqlite"
//	    _ "github.com/mattn/go-sqlite3"
//	    "embed"
//	)
//
//	//go:embed schema.sql
//	var schemaSQL string
//
//	func main() {
//	    db, err := autosqlite.Open(schemaSQL, "myapp.db")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    defer db.Close()
//	    // ...
//	}
package autosqlite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gofrs/flock"
	_ "github.com/mattn/go-sqlite3"
)

// SchemaVersion represents the version information for a schema
type SchemaVersion struct {
	Version   int    // Numeric version (optional, for explicit versioning)
	Hash      string // SHA256 hash of the schema
	Timestamp string // When this version was applied
}

const versionTableName = "_autosqlite_version"

// Open creates or migrates a SQLite database at dbPath using the provided schema SQL.
// If the database does not exist, it is created. If it exists and the schema is unchanged,
// the database is opened as-is. If the schema has changed, a migration is performed and
// the previous database file is backed up with a ".backup" extension.
//
// Returns a *sql.DB handle or an error.
func Open(schema, dbPath string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); err == nil {
		if SchemasEqual(schema, dbPath) {
			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open existing database: %w", err)
			}
			return db, nil
		}

		// Check if this would be a backward migration
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open database for version check: %w", err)
		}
		defer db.Close()

		isForward, err := isForwardMigration(db, schema)
		if err != nil {
			return nil, fmt.Errorf("failed to check migration direction: %w", err)
		}

		if !isForward {
			return nil, fmt.Errorf("backward migration detected: this is not allowed to prevent data loss. If you need to downgrade, clear out the _autosqlite_version table")
		}

		return Migrate(schema, dbPath)
	}

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	// Record the initial schema version
	version := &SchemaVersion{
		Version: 1,
		Hash:    calculateSchemaHash(schema),
	}

	if err := recordSchemaVersion(db, version, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to record schema version: %w", err)
	}

	return db, nil
}

// Migrate migrates an existing SQLite database at dbPath to the provided schema.
// It creates a backup with a ".backup" extension, migrates data for common columns,
// and atomically replaces the old database.
//
// Returns a *sql.DB handle or an error.
func Migrate(schema, dbPath string) (*sql.DB, error) {
	backupPath := dbPath + ".backup"
	newDbPath := dbPath + ".tmp"

	// Lock using the database path, not the tmp path
	lockPath := dbPath + ".migration.lock"
	tmpLock := flock.New(lockPath)
	if err := tmpLock.Lock(); err != nil {
		return nil, fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		tmpLock.Unlock()
		os.Remove(lockPath) // Clean up lock file
	}()

	// Re-check schema after acquiring the lock
	if SchemasEqual(schema, dbPath) {
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open existing database: %w", err)
		}
		return db, nil
	}

	// Re-check for backward migration after acquiring the lock
	dbCheck, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database for version check after lock: %w", err)
	}
	defer dbCheck.Close()
	isForward, err := isForwardMigration(dbCheck, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to check migration direction after lock: %w", err)
	}
	if !isForward {
		return nil, fmt.Errorf("backward migration detected after lock: this is not allowed to prevent data loss. If you need to downgrade, clear out the _autosqlite_version table")
	}

	if err := copyFile(dbPath, backupPath); err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}

	db, err := MigrateToNewFile(schema, dbPath, newDbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate to new file: %w", err)
	}
	db.Close()

	if err := os.Rename(newDbPath, dbPath); err != nil {
		return nil, fmt.Errorf("failed to rename new database: %w", err)
	}

	// Open the migrated database and record the new schema version
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open migrated database: %w", err)
	}

	// Get current version to increment it
	currentVersion, err := getCurrentSchemaVersion(db)
	nextVersion := 1
	if currentVersion != nil {
		nextVersion = currentVersion.Version + 1
	}

	// Record the new schema version
	version := &SchemaVersion{
		Version: nextVersion,
		Hash:    calculateSchemaHash(schema),
	}

	if err := recordSchemaVersion(db, version, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to record schema version: %w", err)
	}

	return db, nil
}

// MigrateToNewFile migrates an existing SQLite database at oldDbPath to the provided schema,
// writing the result to newDbPath. It migrates data for common columns and tables.
//
// Returns a *sql.DB handle to the new database or an error.
func MigrateToNewFile(schema, oldDbPath string, newDbPath string) (*sql.DB, error) {
	oldDB, err := sql.Open("sqlite3", oldDbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open existing database: %w", err)
	}
	defer oldDB.Close()

	newDB, err := sql.Open("sqlite3", newDbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary database: %w", err)
	}

	if _, err := newDB.Exec(schema); err != nil {
		newDB.Close()
		os.Remove(newDbPath)
		return nil, fmt.Errorf("failed to execute new schema: %w", err)
	}

	// Copy _autosqlite_version table if it exists
	row := oldDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", versionTableName)
	var tableName string
	if err := row.Scan(&tableName); err == nil && tableName == versionTableName {
		// Create the version table in the new DB
		if err := createVersionTable(newDB); err != nil {
			newDB.Close()
			os.Remove(newDbPath)
			return nil, fmt.Errorf("failed to create version table in new DB: %w", err)
		}
		// Copy all rows
		rows, err := oldDB.Query("SELECT version, hash, timestamp, schema_sql FROM " + versionTableName)
		if err != nil {
			newDB.Close()
			os.Remove(newDbPath)
			return nil, fmt.Errorf("failed to query version table: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var version int
			var hash, ts, schemaSQL string
			if err := rows.Scan(&version, &hash, &ts, &schemaSQL); err != nil {
				newDB.Close()
				os.Remove(newDbPath)
				return nil, fmt.Errorf("failed to scan version row: %w", err)
			}
			_, err := newDB.Exec("INSERT INTO "+versionTableName+" (version, hash, timestamp, schema_sql) VALUES (?, ?, ?, ?)", version, hash, ts, schemaSQL)
			if err != nil {
				newDB.Close()
				os.Remove(newDbPath)
				return nil, fmt.Errorf("failed to insert version row: %w", err)
			}
		}
	}

	oldTables, err := GetTables(oldDB)
	if err != nil {
		newDB.Close()
		os.Remove(newDbPath)
		return nil, fmt.Errorf("failed to get tables from old database: %w", err)
	}

	newTables, err := GetTables(newDB)
	if err != nil {
		newDB.Close()
		os.Remove(newDbPath)
		return nil, fmt.Errorf("failed to get tables from new database: %w", err)
	}

	for _, tableName := range newTables {
		if slices.Contains(oldTables, tableName) {
			if err := MigrateTable(oldDB, newDB, tableName); err != nil {
				newDB.Close()
				os.Remove(newDbPath)
				return nil, fmt.Errorf("failed to migrate table %s: %w", tableName, err)
			}
		}
	}

	return newDB, nil
}

// SchemasEqual compares the provided schema with the existing database schema at dbPath.
// Returns true if the schemas are equivalent (same tables, columns, triggers, indexes, and views).
func SchemasEqual(schema, dbPath string) bool {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return false
	}
	defer db.Close()

	dbSchema, err := getFullSchema(db)
	if err != nil {
		return false
	}

	// Use in-memory database for temporary schema comparison
	tempDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return false
	}
	defer tempDB.Close()

	// Always create the _autosqlite_version table in the temp DB
	if err := createVersionTable(tempDB); err != nil {
		return false
	}

	if _, err := tempDB.Exec(schema); err != nil {
		return false
	}

	tempSchema, err := getFullSchema(tempDB)
	if err != nil {
		return false
	}

	if len(dbSchema) != len(tempSchema) {
		return false
	}
	for i := range dbSchema {
		if dbSchema[i] != tempSchema[i] {
			return false
		}
	}
	return true
}

// getFullSchema returns a sorted, normalized list of all schema SQL statements for tables, indexes, triggers, and views.
func getFullSchema(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT type, name, sql FROM sqlite_master WHERE type IN ('table','index','trigger','view') AND name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schema []string
	for rows.Next() {
		var typ, name, sqlStmt string
		if err := rows.Scan(&typ, &name, &sqlStmt); err != nil {
			return nil, err
		}
		// Normalize whitespace
		sqlStmt = strings.TrimSpace(sqlStmt)
		schema = append(schema, fmt.Sprintf("%s|%s|%s", typ, name, sqlStmt))
	}
	return schema, rows.Err()
}

// GetTables returns a list of user table names in the database (ignores _autosqlite_version).
func GetTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		if tableName == versionTableName {
			continue
		}
		tables = append(tables, tableName)
	}
	return tables, rows.Err()
}

// MigrateTable migrates data from old table to new table, copying only common columns.
// Returns an error if migration fails.
func MigrateTable(oldDB, newDB *sql.DB, tableName string) error {
	oldColumns, err := GetColumns(oldDB, tableName)
	if err != nil {
		return err
	}

	newColumns, err := GetColumns(newDB, tableName)
	if err != nil {
		return err
	}

	commonColumns := FindCommonColumns(oldColumns, newColumns)
	if len(commonColumns) == 0 {
		return nil // No common columns, skip migration
	}

	selectQuery := fmt.Sprintf("SELECT %s FROM %s", strings.Join(commonColumns, ", "), tableName)
	rows, err := oldDB.Query(selectQuery)
	if err != nil {
		return err
	}
	defer rows.Close()

	placeholders := make([]string, len(commonColumns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertQuery := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName, strings.Join(commonColumns, ", "), strings.Join(placeholders, ", "))

	tx, err := newDB.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(insertQuery)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		values := make([]interface{}, len(commonColumns))
		valuePtrs := make([]interface{}, len(commonColumns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			tx.Rollback()
			return err
		}

		if _, err := stmt.Exec(values...); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// GetColumns returns a list of column names for a table.
func GetColumns(db *sql.DB, tableName string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var index int
		var name, typ, notNull string
		var defaultValue, pk sql.NullString
		if err := rows.Scan(&index, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

// FindCommonColumns returns columns that exist in both old and new tables.
func FindCommonColumns(oldColumns, newColumns []string) []string {
	oldSet := make(map[string]bool)
	for _, col := range oldColumns {
		oldSet[col] = true
	}

	var common []string
	for _, col := range newColumns {
		if oldSet[col] {
			common = append(common, col)
		}
	}
	return common
}

// copyFile copies a file from src to dst using io.Copy.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}

// calculateSchemaHash returns a SHA256 hash of the normalized schema
func calculateSchemaHash(schema string) string {
	// Normalize schema by removing comments and extra whitespace
	normalized := normalizeSchema(schema)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// normalizeSchema removes comments and normalizes whitespace for consistent hashing
func normalizeSchema(schema string) string {
	lines := strings.Split(schema, "\n")
	var normalized []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		normalized = append(normalized, line)
	}

	return strings.Join(normalized, " ")
}

// getCurrentSchemaVersion retrieves the current schema version from the database
func getCurrentSchemaVersion(db *sql.DB) (*SchemaVersion, error) {
	// Check if version table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", versionTableName)
	var tableName string
	if err := row.Scan(&tableName); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No version table means no version tracking
		}
		return nil, err
	}

	// Get current version (order by version DESC, not timestamp)
	row = db.QueryRow("SELECT version, hash, timestamp FROM " + versionTableName + " ORDER BY version DESC LIMIT 1")
	var version SchemaVersion
	if err := row.Scan(&version.Version, &version.Hash, &version.Timestamp); err != nil {
		return nil, err
	}

	return &version, nil
}

// createVersionTable creates the version tracking table
func createVersionTable(db *sql.DB) error {
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version INTEGER,
			hash TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			schema_sql TEXT
		)`, versionTableName)

	_, err := db.Exec(createTableSQL)
	return err
}

// recordSchemaVersion records the current schema version in the database
func recordSchemaVersion(db *sql.DB, version *SchemaVersion, schemaSQL string) error {
	if err := createVersionTable(db); err != nil {
		return err
	}

	insertSQL := fmt.Sprintf("INSERT INTO %s (version, hash, timestamp, schema_sql) VALUES (?, ?, datetime('now'), ?)", versionTableName)
	_, err := db.Exec(insertSQL, version.Version, version.Hash, schemaSQL)
	return err
}

// isForwardMigration checks if the new schema represents a forward migration
// Returns true if migration is allowed, false if it would be a backward migration
func isForwardMigration(db *sql.DB, newSchema string) (bool, error) {
	currentVersion, err := getCurrentSchemaVersion(db)
	if err != nil {
		return false, err
	}

	if currentVersion == nil {
		return true, nil
	}

	newHash := calculateSchemaHash(newSchema)

	if currentVersion.Hash == newHash {
		return true, nil
	}

	row := db.QueryRow("SELECT COUNT(*) FROM "+versionTableName+" WHERE hash = ?", newHash)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}

	if count > 0 {
		return false, nil
	}

	return true, nil
}
