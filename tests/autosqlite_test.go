package tests

import (
	"database/sql"
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

func tempDBPath(t *testing.T) string {
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}
