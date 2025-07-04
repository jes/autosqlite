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

// Open creates a new SQLite database from a schema file.
// If the database file already exists, it migrates the existing data to the new schema.
// If the database doesn't exist, it creates it using the provided schema.
func Open(schema, dbPath string) (*sql.DB, error) {
	// Check if database already exists
	if _, err := os.Stat(dbPath); err == nil {
		// Check if schemas are identical to avoid unnecessary migration
		if schemasEqual(schema, dbPath) {
			// Schemas are the same, just open the existing database
			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open existing database: %w", err)
			}
			return db, nil
		}
		return Migrate(schema, dbPath)
	}

	// Ensure the directory for the database file exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open the database (this will create it if it doesn't exist)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Execute the schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	return db, nil
}

// schemasEqual compares the provided schema with the existing database schema
func schemasEqual(schema, dbPath string) bool {
	// Open existing database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return false
	}
	defer db.Close()

	// Get existing tables
	existingTables, err := getTables(db)
	if err != nil {
		return false
	}

	// Create temporary database with new schema
	tempPath := dbPath + ".schema_check"
	tempDB, err := sql.Open("sqlite3", tempPath)
	if err != nil {
		return false
	}
	defer func() {
		tempDB.Close()
		os.Remove(tempPath)
	}()

	// Execute new schema on temporary database
	if _, err := tempDB.Exec(schema); err != nil {
		return false
	}

	// Get new tables
	newTables, err := getTables(tempDB)
	if err != nil {
		return false
	}

	// Check if table lists are identical
	if !slices.Equal(existingTables, newTables) {
		return false
	}

	// Compare each table's structure
	for _, tableName := range existingTables {
		if !tableStructuresEqual(db, tempDB, tableName) {
			return false
		}
	}

	return true
}

// tableStructuresEqual compares the structure of a table between two databases
func tableStructuresEqual(db1, db2 *sql.DB, tableName string) bool {
	columns1, err := getTableInfo(db1, tableName)
	if err != nil {
		return false
	}

	columns2, err := getTableInfo(db2, tableName)
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

// getTableInfo returns detailed table information as strings for comparison
func getTableInfo(db *sql.DB, tableName string) ([]string, error) {
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

		// Create a string representation of the column for comparison
		colStr := fmt.Sprintf("%s:%s:%s:%s:%s",
			name, typ, notNull,
			defaultValue.String, pk.String)
		columns = append(columns, colStr)
	}
	return columns, rows.Err()
}

func Migrate(schema, dbPath string) (*sql.DB, error) {
	// Create a backup of the database
	backupPath := dbPath + ".backup"
	if err := copyFile(dbPath, backupPath); err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}

	newDbPath := dbPath + ".tmp"

	// Migrate to a new file
	db, err := MigrateToNewFile(schema, dbPath, newDbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate to new file: %w", err)
	}
	db.Close()

	// Replace old database with new one
	if err := os.Rename(newDbPath, dbPath); err != nil {
		return nil, fmt.Errorf("failed to rename new database: %w", err)
	}

	// Reopen the DB from the new path just in case SQLite cares about the exact path
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open migrated database: %w", err)
	}

	return db, nil
}

// Migrate handles the migration of an existing database to a new schema
func MigrateToNewFile(schema, oldDbPath string, newDbPath string) (*sql.DB, error) {
	// Open existing database
	oldDB, err := sql.Open("sqlite3", oldDbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open existing database: %w", err)
	}
	defer oldDB.Close()

	// Create temporary database with new schema
	newDB, err := sql.Open("sqlite3", newDbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary database: %w", err)
	}

	// Execute new schema on temporary database
	if _, err := newDB.Exec(schema); err != nil {
		newDB.Close()
		os.Remove(newDbPath)
		return nil, fmt.Errorf("failed to execute new schema: %w", err)
	}

	// Get tables from both databases
	oldTables, err := getTables(oldDB)
	if err != nil {
		newDB.Close()
		os.Remove(newDbPath)
		return nil, fmt.Errorf("failed to get tables from old database: %w", err)
	}

	newTables, err := getTables(newDB)
	if err != nil {
		newDB.Close()
		os.Remove(newDbPath)
		return nil, fmt.Errorf("failed to get tables from new database: %w", err)
	}

	// Migrate data for common tables
	for _, tableName := range newTables {
		if slices.Contains(oldTables, tableName) {
			if err := migrateTable(oldDB, newDB, tableName); err != nil {
				newDB.Close()
				os.Remove(newDbPath)
				return nil, fmt.Errorf("failed to migrate table %s: %w", tableName, err)
			}
		}
	}

	return newDB, nil
}

// getTables returns a list of table names in the database
func getTables(db *sql.DB) ([]string, error) {
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

// migrateTable migrates data from old table to new table
func migrateTable(oldDB, newDB *sql.DB, tableName string) error {
	// Get column information from both tables
	oldColumns, err := getColumns(oldDB, tableName)
	if err != nil {
		return err
	}

	newColumns, err := getColumns(newDB, tableName)
	if err != nil {
		return err
	}

	// Find common columns
	commonColumns := findCommonColumns(oldColumns, newColumns)
	if len(commonColumns) == 0 {
		return nil // No common columns, skip migration
	}

	// Build SELECT query for old table
	selectQuery := fmt.Sprintf("SELECT %s FROM %s", strings.Join(commonColumns, ", "), tableName)
	rows, err := oldDB.Query(selectQuery)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Build INSERT query for new table
	placeholders := make([]string, len(commonColumns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertQuery := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName, strings.Join(commonColumns, ", "), strings.Join(placeholders, ", "))

	// Migrate data row by row
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

// getColumns returns a list of column names for a table
func getColumns(db *sql.DB, tableName string) ([]string, error) {
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

// findCommonColumns returns columns that exist in both old and new tables
func findCommonColumns(oldColumns, newColumns []string) []string {
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

// copyFile copies a file from src to dst
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
