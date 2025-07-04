# Edge Cases and Potential Issues - TODO

## Schema Changes
- Column and table renames not handled - The migration system only copies data for columns/tables with matching names. Renamed columns or tables will be treated as deleted from the old schema and new in the new schema, resulting in data loss.

## Data Type Compatibility
- NULL value handling in NOT NULL columns - No validation of existing NULL values against new NOT NULL constraints

## Constraint Violations
- New UNIQUE constraints may conflict with existing data - No validation of existing data against new constraints
- New NOT NULL constraints may fail on existing NULL values - No validation or default value handling
- New FOREIGN KEY constraints may reference non-existent data - No validation of foreign key relationships
- CHECK constraints may be violated by existing data - No validation of existing data against new constraints

## Schema Dependencies
- Foreign key relationships require specific migration order - No dependency analysis or ordering
- Circular dependencies between tables - No detection or handling of circular dependencies
- Views, triggers, and custom functions not preserved - Only handles tables and columns
- Indexes need to be recreated - Indexes are not preserved during migration

## SQLite-Specific Issues
- SQLite version compatibility between old and new schemas
- WAL mode and journal file handling
- Virtual tables and extensions
- Custom collations and functions

## User Experience
- Progress reporting for long migrations
- Dry-run mode for testing
- Migration log and audit trail

## Recommended Next Steps (Easiest to Hardest)
1. Migration Logging - Add logging to track what changes are being made during migration
2. Dry-Run Mode - Add a flag to simulate migrations without making changes
3. Progress Reporting - Add progress indicators for large dataset migrations
4. Constraint Validation - Add validation for NOT NULL and UNIQUE constraints before migration
5. Index Preservation - Preserve and recreate indexes during migration
6. Foreign Key Validation - Validate foreign key relationships before applying constraints
7. Column/Table Rename Detection - Add logic to detect and handle renamed columns and tables

## Minor Improvements
- Make temporary files with better filenames