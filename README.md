# Autosqlite

A Go module for creating SQLite databases from schema files and automatically
handling migrations when the schema changes.

- Creates SQLite databases from SQL schema strings
- Automatic schema migration
- Returns a `*sql.DB` handle for immediate use
- Skips migration if schema is unchanged
- Avoids backwards migrations
- Creates backups before migration
- Uses the `mattn/go-sqlite3` driver

When a schema change is found, Autosqlite creates a database from the new schema,
and copies all of the data from the old database where table and column names are
equal, and then renames the new database on top of the old one.

Autosqlite creates a table called `_autosqlite_version`, listing schemas that
have been applied by Autosqlite. If Autosqlite finds itself trying to apply a
schema that is older than the newest version that has been applied (for example
because you launched an old version of your program) it bails out instead of
reverting to the old schema.

## Caveats

Schema migration might not do the right thing in some circumstances:

 - Renaming a column or table is indistinguishable from deleting the old one
   and adding a new one, so data loss is guaranteed if you rename columns or
   tables
 - If another program has the old database file open while you try to migrate
   it, you might lose data
 - If you use foreign key constraints, Autosqlite won't necessarily
   re-populate the tables in the right order, leading to migration failures
 - If you introduce for example a `NOT NULL` constraint on a column that
   previously had `NULL` values then migration will fail
 - You can't revert to an old schema, because of the backwards migration
   prevention; you'd need to make some other trivial change to the schema

## Recommended usage

Use Autosqlite to save yourself time while developing a new application. **DON'T**
attempt to use Autosqlite to handle important data. For handling important data
you should use a more robust method to deploy database schema migrations.

If you do want to use Autosqlite to handle schema migrations automatically, the
safest way is to use the CLI tool, and manually run it every time you need to
update the schema.

The more dangerous, but more convenient, way is to embed the schema into your
program using `go:embed` and use `autosqlite.Open()` to open the database. It
will automatically migrate the schema whenever a new version is found.

## CLI Tool

Autosqlite includes a command-line tool for database management.

### Installing the CLI Tool

**Option 1: Install globally (recommended)**
```bash
go install github.com/jes/autosqlite/cmd/autosqlite@latest
```

**Option 2: Add as a project tool**
```bash
go get -tool github.com/jes/autosqlite/cmd/autosqlite
```
Then run with: `go tool autosqlite`

**Option 3: Build locally**
```bash
git clone https://github.com/jes/autosqlite.git
cd autosqlite
go build -o autosqlite cmd/autosqlite/main.go
```

### CLI Usage

```bash
# Validate a schema file
autosqlite -validate -schema schema.sql

# Test migration without applying changes
autosqlite -dry-run -schema schema.sql -db app.db

# Migrate database in place (creates backup)
autosqlite -schema schema.sql -db app.db -in-place

# Create new database with migrated schema
autosqlite -schema schema.sql -db app.db -new-db app_v2.db

# Add verbose output to any command
autosqlite -schema schema.sql -db app.db -in-place -verbose
```

### CLI Commands

- `-validate -schema <file>` - Validate schema syntax
- `-dry-run -schema <file> -db <file>` - Test migration without applying
- `-schema <file> -db <file> -in-place` - Migrate database in place
- `-schema <file> -db <file> -new-db <file>` - Create new database with migrated schema
- `-verbose` - Show detailed tool mation

## Package Usage

```bash
go get github.com/jes/autosqlite
```

Put your schema in `schema.sql` and embed it in a string in your program:

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
