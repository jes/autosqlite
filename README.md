# AutoSQLite

A Go module for creating SQLite databases from schema files.

## Features

- Creates SQLite databases from SQL schema strings
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
    "os"
    "github.com/jes/autosqlite"
)

func main() {
    schema, err := os.ReadFile("schema.sql")
    if err != nil {
        log.Fatalf("Failed to read schema: %v", err)
    }
    db, err := autosqlite.Open(string(schema), "myapp.db")
    if err != nil {
        log.Fatalf("Failed to create database: %v", err)
    }
    defer db.Close()
    // Use the database...
    // db is a *db.SQL
}
```

## Embedding a Schema with go:embed

You can embed your schema at compile time using Go's `embed` package:

```go
import (
    "github.com/jes/autosqlite"
    _ "github.com/mattn/go-sqlite3"
    "embed"
    "log"
)

//go:embed schema.sql
var schema string

func main() {
    db, err := autosqlite.Open(schema, "myapp.db")
    if err != nil {
        log.Fatalf("Failed to create database: %v", err)
    }
    defer db.Close()
    // Use the database...
}
```

## Function Signature

```go
func Open(schema string, dbPath string) (*sql.DB, error)
```

### Parameters

- `schema`: SQL schema as a string
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

See the `cmd/autosqlite/` directory for a complete working example.

## Dependencies

- `github.com/mattn/go-sqlite3` - SQLite driver for Go 