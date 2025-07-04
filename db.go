package autosqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// CreateDBFromSchema creates a new SQLite database from a schema file.
// If the database file already exists, it returns an error.
// If the database doesn't exist, it creates it using the provided schema.
func CreateDBFromSchema(schemaPath, dbPath string) (*sql.DB, error) {
	// Check if database already exists
	if _, err := os.Stat(dbPath); err == nil {
		return nil, fmt.Errorf("database file already exists: %s", dbPath)
	}

	// Ensure the directory for the database file exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Read the schema file
	schemaContent, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
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
	if _, err := db.Exec(string(schemaContent)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	return db, nil
}
