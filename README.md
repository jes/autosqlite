# AutoSQLite

A Go module for creating SQLite databases from schema files.

## Features

- Creates SQLite databases from SQL schema strings
- Automatic schema migration with data preservation
- Returns a `*sql.DB` handle for immediate use
- Efficient: skips migration if schema is unchanged
- Automatically creates necessary directories
- Creates backups before migration
- Uses the `mattn/go-sqlite3` driver

## Installation

```bash
go get github.com/jes/autosqlite
```

## Usage

```go
package main

import (
    "log"
    "github.com/jes/autosqlite"
    _ "github.com/mattn/go-sqlite3"
    "embed"
)

//go:embed schema.sql
var schemaSQL string

func main() {
    db, err := autosqlite.Open(schemaSQL, "myapp.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    // Use the database...
}
```

## Function Signatures

### Open
```go
func Open(schema string, dbPath string) (*sql.DB, error)
```
Creates or migrates a SQLite database at dbPath using the provided schema SQL.
If the database does not exist, it is created. If it exists and the schema is unchanged,
the database is opened as-is. If the schema has changed, a migration is performedand
the previous database file is backed up with a ".backup" extension.

Returns a *sql.DB handle or an error.

### Migrate
```go
func Migrate(schema string, dbPath string) (*sql.DB, error)
```
Migrates an existing SQLite database at dbPath to the provided schema.
It creates a backup with a ".backup" extension, migrates data for common columns,
and atomically replaces the old database.

Returns a *sql.DB handle or an error.

### MigrateToNewFile
```go
func MigrateToNewFile(schema string, oldDbPath string, newDbPath string) (*sql.DB, error)
```
Migrates an existing SQLite database at oldDbPath to the provided schema,
writing the result to newDbPath. It migrates data for common columns and tables.

Returns a *sql.DB handle to the new database or an error.

## Example

See the `cmd/autosqlite/` directory for a complete working example.

## Dependencies

- `github.com/mattn/go-sqlite3` - SQLite driver for Go 