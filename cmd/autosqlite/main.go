package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jes/autosqlite"
)

func main() {
	schemaPath := flag.String("schema", "", "Path to schema.sql file")
	dbPath := flag.String("db", "", "Path to SQLite database file to create")
	flag.Parse()

	if *schemaPath == "" || *dbPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -schema <schema.sql> -db <dbfile>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	schema, err := os.ReadFile(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := autosqlite.Open(string(schema), *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("Database created at %s\n", *dbPath)
}
