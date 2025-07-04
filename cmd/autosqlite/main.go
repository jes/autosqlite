package main

import (
	"fmt"
	"os"

	"github.com/jes/autosqlite"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <schema.sql> <dbfile>\n", os.Args[0])
		os.Exit(1)
	}

	schemaPath := os.Args[1]
	dbPath := os.Args[2]

	db, err := autosqlite.CreateDBFromSchema(schemaPath, dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("Database created at %s\n", dbPath)
}
