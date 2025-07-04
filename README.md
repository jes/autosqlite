# AutoSQLite

A Go module for creating SQLite databases from schema files.

## Features

- Creates SQLite databases from SQL schema files
- Returns a `*sql.DB` handle for immediate use
- Ensures database doesn't already exist (returns error if it does)
- Automatically creates necessary directories
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
)

func main() {
    // Create database from schema
    db, err := autosqlite.CreateDBFromSchema("schema.sql", "myapp.db")
    if err != nil {
        log.Fatalf("Failed to create database: %v", err)
    }
    defer db.Close()
    
    // Use the database...
}
```

## Function Signature

```go
func CreateDBFromSchema(schemaPath, dbPath string) (*sql.DB, error)
```

### Parameters

- `schemaPath`: Path to the SQL schema file
- `dbPath`: Path where the SQLite database should be created

### Return Value

- `*sql.DB`: Database handle for immediate use
- `error`: Error if database creation fails or if database already exists

## Behavior

- If the database file already exists, returns an error
- If the database doesn't exist, creates it using the provided schema
- Automatically creates any necessary directories for the database file
- Returns a ready-to-use database handle

## Example

See the `example/` directory for a complete working example.

## Dependencies

- `github.com/mattn/go-sqlite3` - SQLite driver for Go 