package tests

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jes/autosqlite"
	_ "github.com/mattn/go-sqlite3"
)

const schemaV1 = `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);`
const schemaV2 = `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);`
const schemaV1WithPosts = `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT); CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT);`
const schemaV2DropName = `CREATE TABLE users (id INTEGER PRIMARY KEY);`
const schemaV2DropPosts = `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);`

func TestCreateNewDB(t *testing.T) {
	dbPath := tempDBPath(t)
	db, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	// Check table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'")
	var name string
	if err := row.Scan(&name); err != nil || name != "users" {
		t.Fatalf("users table not created: %v", err)
	}
}

func TestMigrationAddsColumn(t *testing.T) {
	dbPath := tempDBPath(t)
	// Create v1
	db, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (name) VALUES ('alice')")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	db.Close()

	// Migrate to v2
	db2, err := autosqlite.Open(schemaV2, dbPath)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	defer db2.Close()

	// Check new column exists
	foundEmail := false
	rows, err := db2.Query("PRAGMA table_info(users)")
	if err != nil {
		t.Fatalf("failed to query table info: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var colName, colType, notnull string
		var dfltValue, pk sql.NullString
		if err := rows.Scan(&cid, &colName, &colType, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("failed to scan table info: %v", err)
		}
		if colName == "email" {
			foundEmail = true
		}
	}
	if !foundEmail {
		t.Fatalf("email column not found after migration")
	}

	// Check old data is preserved
	row := db2.QueryRow("SELECT name FROM users WHERE id=1")
	var name string
	if err := row.Scan(&name); err != nil || name != "alice" {
		t.Fatalf("old data not preserved: %v", err)
	}
}

func TestMigrationDeletesColumn(t *testing.T) {
	dbPath := tempDBPath(t)
	// Create v2 (with name column)
	db, err := autosqlite.Open(schemaV2, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (name, email) VALUES ('bob', 'bob@example.com')")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	db.Close()

	// Migrate to v2DropName (drops name column)
	db2, err := autosqlite.Open(schemaV2DropName, dbPath)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	defer db2.Close()

	// Check name column is gone
	rows, err := db2.Query("PRAGMA table_info(users)")
	if err != nil {
		t.Fatalf("failed to query table info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var colName, colType, notnull string
		var dfltValue, pk sql.NullString
		if err := rows.Scan(&cid, &colName, &colType, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("failed to scan table info: %v", err)
		}
		if colName == "name" {
			t.Fatalf("name column should have been deleted")
		}
	}

	// Check data is preserved for id
	row := db2.QueryRow("SELECT id FROM users WHERE id=1")
	var id int
	if err := row.Scan(&id); err != nil || id != 1 {
		t.Fatalf("id not preserved: %v", err)
	}
}

func TestMigrationAddsTable(t *testing.T) {
	dbPath := tempDBPath(t)
	// Create v1 (users only)
	db, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	db.Close()

	// Migrate to v1WithPosts (adds posts table)
	db2, err := autosqlite.Open(schemaV1WithPosts, dbPath)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	defer db2.Close()

	// Check posts table exists
	row := db2.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='posts'")
	var name string
	if err := row.Scan(&name); err != nil || name != "posts" {
		t.Fatalf("posts table not created: %v", err)
	}
}

func TestMigrationDeletesTable(t *testing.T) {
	dbPath := tempDBPath(t)
	// Create v1WithPosts (users and posts)
	db, err := autosqlite.Open(schemaV1WithPosts, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	_, err = db.Exec("INSERT INTO posts (title) VALUES ('hello')")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	db.Close()

	// Migrate to v2DropPosts (drops posts table)
	db2, err := autosqlite.Open(schemaV2DropPosts, dbPath)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	defer db2.Close()

	// Check posts table is gone
	row := db2.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='posts'")
	var name string
	if err := row.Scan(&name); err == nil {
		t.Fatalf("posts table should have been deleted")
	}
}

func TestIdenticalSchemaSkipMigration(t *testing.T) {
	dbPath := tempDBPath(t)

	// Create database with schemaV1
	db1, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	_, err = db1.Exec("INSERT INTO users (name) VALUES ('test')")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	db1.Close()

	// Open with same schema - should skip migration
	db2, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to open db with identical schema: %v", err)
	}
	defer db2.Close()

	// Verify data is still there (no migration occurred)
	row := db2.QueryRow("SELECT name FROM users WHERE id=1")
	var name string
	if err := row.Scan(&name); err != nil || name != "test" {
		t.Fatalf("data not preserved: %v", err)
	}

	// Verify no backup file was created (since no migration occurred)
	backupPath := dbPath + ".backup"
	if _, err := os.Stat(backupPath); err == nil {
		t.Fatalf("backup file was created unnecessarily")
	}
}

// Direct function tests
func TestMigrateFunction(t *testing.T) {
	dbPath := tempDBPath(t)

	// Create initial database
	db, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (name) VALUES ('test')")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	db.Close()

	// Test Migrate function directly
	db2, err := autosqlite.Migrate(schemaV2, dbPath)
	if err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	defer db2.Close()

	// Verify migration worked
	row := db2.QueryRow("SELECT name FROM users WHERE id=1")
	var name string
	if err := row.Scan(&name); err != nil || name != "test" {
		t.Fatalf("data not preserved after migration: %v", err)
	}

	// Check backup was created
	backupPath := dbPath + ".backup"
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
}

func TestMigrateToNewFile(t *testing.T) {
	oldDbPath := tempDBPath(t)
	newDbPath := tempDBPath(t) + ".new"

	// Create initial database
	db, err := autosqlite.Open(schemaV1, oldDbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (name) VALUES ('test')")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	db.Close()

	// Test MigrateToNewFile function
	db2, err := autosqlite.MigrateToNewFile(schemaV2, oldDbPath, newDbPath)
	if err != nil {
		t.Fatalf("migrate to new file failed: %v", err)
	}
	defer db2.Close()

	// Verify new database has migrated data
	row := db2.QueryRow("SELECT name FROM users WHERE id=1")
	var name string
	if err := row.Scan(&name); err != nil || name != "test" {
		t.Fatalf("data not preserved in new file: %v", err)
	}

	// Verify old database still exists and unchanged
	db3, err := sql.Open("sqlite3", oldDbPath)
	if err != nil {
		t.Fatalf("failed to open old db: %v", err)
	}
	defer db3.Close()

	row = db3.QueryRow("SELECT name FROM users WHERE id=1")
	if err := row.Scan(&name); err != nil || name != "test" {
		t.Fatalf("old database was modified: %v", err)
	}
}

func TestSchemasEqual(t *testing.T) {
	dbPath := tempDBPath(t)

	// Create database with schemaV1
	db, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	db.Close()

	// Test identical schema
	if !autosqlite.SchemasEqual(schemaV1, dbPath) {
		t.Fatalf("identical schemas should be equal")
	}

	// Test different schema
	if autosqlite.SchemasEqual(schemaV2, dbPath) {
		t.Fatalf("different schemas should not be equal")
	}
}

// Utility function tests
func TestGetTables(t *testing.T) {
	dbPath := tempDBPath(t)
	db, err := autosqlite.Open(schemaV1WithPosts, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	tables, err := autosqlite.GetTables(db)
	if err != nil {
		t.Fatalf("GetTables failed: %v", err)
	}

	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}

	expected := []string{"users", "posts"}
	for _, table := range expected {
		found := false
		for _, t := range tables {
			if t == table {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("table %s not found", table)
		}
	}
}

func TestGetColumns(t *testing.T) {
	dbPath := tempDBPath(t)
	db, err := autosqlite.Open(schemaV2, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	columns, err := autosqlite.GetColumns(db, "users")
	if err != nil {
		t.Fatalf("GetColumns failed: %v", err)
	}

	expected := []string{"id", "name", "email"}
	if len(columns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(columns))
	}

	for _, col := range expected {
		found := false
		for _, c := range columns {
			if c == col {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("column %s not found", col)
		}
	}
}

func TestFindCommonColumns(t *testing.T) {
	oldCols := []string{"id", "name", "email"}
	newCols := []string{"id", "name", "phone"}

	common := autosqlite.FindCommonColumns(oldCols, newCols)
	expected := []string{"id", "name"}

	if len(common) != len(expected) {
		t.Fatalf("expected %d common columns, got %d", len(expected), len(common))
	}

	for _, col := range expected {
		found := false
		for _, c := range common {
			if c == col {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("common column %s not found", col)
		}
	}
}

// Error case tests
func TestInvalidSchema(t *testing.T) {
	dbPath := tempDBPath(t)

	// Test with invalid SQL
	_, err := autosqlite.Open("INVALID SQL", dbPath)
	if err == nil {
		t.Fatalf("should fail with invalid SQL")
	}
}

func TestEmptySchema(t *testing.T) {
	dbPath := tempDBPath(t)

	// Test with empty schema - should create an empty database
	db, err := autosqlite.Open("", dbPath)
	if err != nil {
		t.Fatalf("empty schema should create empty database: %v", err)
	}
	defer db.Close()

	// Verify database was created but has no tables
	tables, err := autosqlite.GetTables(db)
	if err != nil {
		t.Fatalf("GetTables failed: %v", err)
	}

	if len(tables) != 0 {
		t.Fatalf("empty schema should create database with no tables, got %d tables", len(tables))
	}
}

func TestNonExistentDatabasePath(t *testing.T) {
	// Test with non-existent database path
	_, err := autosqlite.Open(schemaV1, "/non/existent/path/db.sqlite")
	if err == nil {
		t.Fatalf("should fail with non-existent path")
	}
}

func TestSchemasEqualWithNonExistentDB(t *testing.T) {
	// Test SchemasEqual with non-existent database
	if autosqlite.SchemasEqual(schemaV1, "/non/existent/db.sqlite") {
		t.Fatalf("should return false for non-existent database")
	}
}

// Edge case tests
func TestComplexSchema(t *testing.T) {
	complexSchema := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE posts (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		content TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	CREATE INDEX idx_posts_user_id ON posts(user_id);
	`

	dbPath := tempDBPath(t)
	db, err := autosqlite.Open(complexSchema, dbPath)
	if err != nil {
		t.Fatalf("failed to create db with complex schema: %v", err)
	}
	defer db.Close()

	// Verify tables were created
	tables, err := autosqlite.GetTables(db)
	if err != nil {
		t.Fatalf("GetTables failed: %v", err)
	}

	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
}

func TestLargeDatasetMigration(t *testing.T) {
	dbPath := tempDBPath(t)

	// Create database with data
	db, err := autosqlite.Open(schemaV1, dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	// Insert many rows
	for i := 0; i < 1000; i++ {
		_, err = db.Exec("INSERT INTO users (name) VALUES (?)", fmt.Sprintf("user%d", i))
		if err != nil {
			t.Fatalf("failed to insert row %d: %v", i, err)
		}
	}
	db.Close()

	// Migrate to new schema
	db2, err := autosqlite.Open(schemaV2, dbPath)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	defer db2.Close()

	// Verify all data was migrated
	var count int
	row := db2.QueryRow("SELECT COUNT(*) FROM users")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 1000 {
		t.Fatalf("expected 1000 rows, got %d", count)
	}
}

func TestConcurrentMigration(t *testing.T) {
	const numGoroutines = 20
	const numIterations = 10

	schema1 := "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);"
	schema2 := "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);"

	for iter := 0; iter < numIterations; iter++ {
		dbPath := tempDBPath(t)

		// Create initial database
		db, err := autosqlite.Open(schema1, dbPath)
		if err != nil {
			t.Fatalf("[%d] failed to create db: %v", iter, err)
		}
		_, err = db.Exec("INSERT INTO users (name) VALUES ('concurrent')")
		if err != nil {
			t.Fatalf("[%d] failed to insert: %v", iter, err)
		}

		// Check that the 'email' column does NOT exist before migration
		columns, err := autosqlite.GetColumns(db, "users")
		if err != nil {
			t.Fatalf("[%d] GetColumns failed before migration: %v", iter, err)
		}
		for _, col := range columns {
			if col == "email" {
				t.Fatalf("[%d] email column should not exist before migration", iter)
			}
		}
		db.Close()

		start := make(chan struct{})
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				<-start
				_, err := autosqlite.Migrate(schema2, dbPath)
				results <- err
			}()
		}

		close(start) // Start all migrations at the same time

		var gotSuccess int
		for i := 0; i < numGoroutines; i++ {
			err := <-results
			if err != nil {
				t.Fatalf("[%d] concurrent migration failed: %v", iter, err)
			}
			gotSuccess++
		}
		if gotSuccess != numGoroutines {
			t.Fatalf("[%d] expected all migrations to succeed, got %d", iter, gotSuccess)
		}

		// Verify the database is correct
		db2, err := autosqlite.Open(schema2, dbPath)
		if err != nil {
			t.Fatalf("[%d] failed to open db after concurrent migration: %v", iter, err)
		}
		defer db2.Close()

		row := db2.QueryRow("SELECT name FROM users WHERE id=1")
		var name string
		if err := row.Scan(&name); err != nil || name != "concurrent" {
			t.Fatalf("[%d] data not preserved after concurrent migration: %v", iter, err)
		}

		// Check that the 'email' column exists
		columns, err = autosqlite.GetColumns(db2, "users")
		if err != nil {
			t.Fatalf("[%d] GetColumns failed: %v", iter, err)
		}
		foundEmail := false
		for _, col := range columns {
			if col == "email" {
				foundEmail = true
				break
			}
		}
		if !foundEmail {
			t.Fatalf("[%d] email column not found after concurrent migration", iter)
		}
	}
}

func tempDBPath(t *testing.T) string {
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}
