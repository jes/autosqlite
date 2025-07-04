package main

import (
	"fmt"
	"log"

	"github.com/jes/autosqlite"
)

func main() {
	// Example schema file path
	schemaPath := "schema.sql"
	dbPath := "example.db"

	// Create database from schema
	db, err := autosqlite.CreateDBFromSchema(schemaPath, dbPath)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	fmt.Println("Database created successfully!")
	fmt.Printf("Database path: %s\n", dbPath)

	// Example: Query to verify the database was created
	var version string
	err = db.QueryRow("SELECT sqlite_version()").Scan(&version)
	if err != nil {
		log.Fatalf("Failed to query database version: %v", err)
	}
	fmt.Printf("SQLite version: %s\n", version)
}
