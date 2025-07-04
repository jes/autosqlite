// Package autosqlite provides automatic SQLite database creation and migration
// from a schema string. It can create a new database, migrate an existing one
// to a new schema, and ensures data is preserved for common columns and tables.
//
// Features:
//   - Automatic schema migration
//   - Data preservation for common columns
//   - Backup and atomic replacement
//   - Skips migration if schema is unchanged
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
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

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

	return db, nil
}

// Migrate migrates an existing SQLite database at dbPath to the provided schema.
// It creates a backup with a ".backup" extension, migrates data for common columns,
// and atomically replaces the old database.
// Returns a *sql.DB handle or an error.
func Migrate(schema, dbPath string) (*sql.DB, error) {
	backupPath := dbPath + ".backup"
	if err := copyFile(dbPath, backupPath); err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}

	newDbPath := dbPath + ".tmp"
	db, err := MigrateToNewFile(schema, dbPath, newDbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate to new file: %w", err)
	}
	db.Close()

	if err := os.Rename(newDbPath, dbPath); err != nil {
		return nil, fmt.Errorf("failed to rename new database: %w", err)
	}
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open migrated database: %w", err)
	}

	return db, nil
}

// MigrateToNewFile migrates an existing SQLite database at oldDbPath to the provided schema,
// writing the result to newDbPath. It migrates data for common columns and tables.
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
// Returns true if the schemas are equivalent (same tables, columns, and properties).
func SchemasEqual(schema, dbPath string) bool {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return false
	}
	defer db.Close()

	existingTables, err := GetTables(db)
	if err != nil {
		return false
	}

	tempPath := dbPath + ".schema_check"
	tempDB, err := sql.Open("sqlite3", tempPath)
	if err != nil {
		return false
	}
	defer func() {
		tempDB.Close()
		os.Remove(tempPath)
	}()

	if _, err := tempDB.Exec(schema); err != nil {
		return false
	}

	newTables, err := GetTables(tempDB)
	if err != nil {
		return false
	}

	if !slices.Equal(existingTables, newTables) {
		return false
	}

	for _, tableName := range existingTables {
		if !TableStructuresEqual(db, tempDB, tableName) {
			return false
		}
	}

	return true
}

// TableStructuresEqual compares the structure of a table between two databases.
// Returns true if the columns and their properties are identical.
func TableStructuresEqual(db1, db2 *sql.DB, tableName string) bool {
	columns1, err := GetTableInfo(db1, tableName)
	if err != nil {
		return false
	}

	columns2, err := GetTableInfo(db2, tableName)
	if err != nil {
		return false
	}

	if len(columns1) != len(columns2) {
		return false
	}

	for i, col1 := range columns1 {
		col2 := columns2[i]
		if col1 != col2 {
			return false
		}
	}

	return true
}

// GetTableInfo returns detailed table information as strings for comparison.
// Each string contains column name, type, nullability, default value, and primary key status.
func GetTableInfo(db *sql.DB, tableName string) ([]string, error) {
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
		colStr := fmt.Sprintf("%s:%s:%s:%s:%s", name, typ, notNull, defaultValue.String, pk.String)
		columns = append(columns, colStr)
	}
	return columns, rows.Err()
}

// GetTables returns a list of table names in the database.
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
