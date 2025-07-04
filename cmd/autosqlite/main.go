package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	"github.com/jes/autosqlite"
)

func main() {
	// Main command flags
	schemaPath := flag.String("schema", "", "Path to schema.sql file")
	dbPath := flag.String("db", "", "Path to SQLite database file")

	// Migration control flags
	inPlace := flag.Bool("in-place", false, "Migrate database in place (creates backup)")
	newDb := flag.String("new-db", "", "Create new database file with migrated schema")

	// Feature flags
	dryRun := flag.Bool("dry-run", false, "Test migration without applying changes")
	validate := flag.Bool("validate", false, "Validate schema syntax only")
	verbose := flag.Bool("verbose", false, "Show detailed migration information")

	flag.Parse()

	// Handle different commands
	switch {
	case *validate:
		validateSchema(*schemaPath)
	case *dryRun:
		dryRunMigration(*schemaPath, *dbPath, *verbose)
	case *schemaPath != "" && *dbPath != "" && (*inPlace || *newDb != ""):
		createOrMigrate(*schemaPath, *dbPath, *inPlace, *newDb, *verbose)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [command] [options]

Commands:
  -validate -schema <file>                    Validate schema syntax
  -dry-run -schema <file> -db <file>          Test migration without applying
  -schema <file> -db <file> -in-place         Migrate database in place
  -schema <file> -db <file> -new-db <file>    Create new database with migrated schema

Options:
  -verbose                                   Show detailed information

Examples:
  %s -validate -schema schema.sql
  %s -dry-run -schema schema.sql -db app.db
  %s -schema schema.sql -db app.db -in-place
  %s -schema schema.sql -db app.db -new-db app_v2.db
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}

func validateSchema(schemaPath string) {
	if schemaPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -schema flag is required for validation\n")
		os.Exit(1)
	}

	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading schema file: %v\n", err)
		os.Exit(1)
	}

	// Try to create a temporary database to validate the schema
	db, err := autosqlite.Open(string(schema), ":memory:")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Schema validation failed: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("✓ Schema is valid\n")
}

func dryRunMigration(schemaPath, dbPath string, verbose bool) {
	if schemaPath == "" || dbPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -schema and -db flags are required for dry-run\n")
		os.Exit(1)
	}

	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading schema file: %v\n", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("Checking if database exists: %s\n", dbPath)
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); err == nil {
		// Database exists, check if migration is needed
		if autosqlite.SchemasEqual(string(schema), dbPath) {
			fmt.Printf("✓ No migration needed - schemas are identical\n")
			return
		}

		if verbose {
			fmt.Printf("Migration would be performed:\n")
			fmt.Printf("  - Backup would be created at %s.backup\n", dbPath)
			fmt.Printf("  - New schema would be applied\n")
			fmt.Printf("  - Data would be migrated\n")
		} else {
			fmt.Printf("Migration would be performed (use -verbose for details)\n")
		}
	} else {
		fmt.Printf("✓ New database would be created with schema\n")
	}
}

func createOrMigrate(schemaPath, dbPath string, inPlace bool, newDbPath string, verbose bool) {
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading schema file: %v\n", err)
		os.Exit(1)
	}

	if verbose {
		if inPlace {
			fmt.Printf("Migrating database in place: %s\n", dbPath)
		} else if newDbPath != "" {
			fmt.Printf("Creating new database with migrated schema: %s -> %s\n", dbPath, newDbPath)
		}
	}

	var db *sql.DB
	var err2 error

	if inPlace {
		// Migrate in place (this will create a backup automatically)
		db, err2 = autosqlite.Open(string(schema), dbPath)
	} else if newDbPath != "" {
		// Create new database with migrated schema
		db, err2 = autosqlite.MigrateToNewFile(string(schema), dbPath, newDbPath)
	} else {
		fmt.Fprintf(os.Stderr, "Error: Either -in-place or -new-db must be specified\n")
		os.Exit(1)
	}

	if err2 != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err2)
		os.Exit(1)
	}
	defer db.Close()

	if verbose {
		fmt.Printf("✓ Database operation completed successfully\n")
	} else {
		if inPlace {
			fmt.Printf("Database migrated in place: %s\n", dbPath)
		} else {
			fmt.Printf("New database created: %s\n", newDbPath)
		}
	}
}
