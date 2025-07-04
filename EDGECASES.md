# Edge Cases and Potential Issues

## ✅ RESOLVED

### SQLite Connection String Handling
- ✅ Query parameters in database paths - **RESOLVED**: Added `extractFilenameFromConnectionString()` function to handle connection strings like "foo.db?_busy_timeout=1000", extracting only the filename for file operations while preserving the full string for database connections

## ⚠️ PARTIALLY ADDRESSED

### Data Type Compatibility
- ⚠️ Data truncation or conversion issues - **LIMITED**: Only occurs during specific operations (arithmetic, comparisons) that require type conversion
- ⚠️ NULL value handling in NOT NULL columns - **NOT HANDLED**: No validation of existing NULL values against new NOT NULL constraints

### Constraint Violations
- ⚠️ New UNIQUE constraints may conflict with existing data - **NOT HANDLED**: No validation of existing data against new constraints
- ⚠️ New NOT NULL constraints may fail on existing NULL values - **NOT HANDLED**: No validation or default value handling
- ⚠️ New FOREIGN KEY constraints may reference non-existent data - **NOT HANDLED**: No validation of foreign key relationships
- ⚠️ CHECK constraints may be violated by existing data - **NOT HANDLED**: No validation of existing data against new constraints

### Schema Dependencies
- ⚠️ Foreign key relationships require specific migration order - **NOT HANDLED**: No dependency analysis or ordering
- ⚠️ Circular dependencies between tables - **NOT HANDLED**: No detection or handling of circular dependencies
- ⚠️ Views, triggers, and custom functions not preserved - **NOT HANDLED**: Only handles tables and columns
- ⚠️ Indexes need to be recreated - **NOT HANDLED**: Indexes are not preserved during migration

## ❌ NOT ADDRESSED

### SQLite-Specific Issues
- ❌ SQLite version compatibility between old and new schemas
- ❌ WAL mode and journal file handling
- ❌ Virtual tables and extensions
- ❌ Custom collations and functions

### User Experience
- ❌ Progress reporting for long migrations
- ❌ Dry-run mode for testing
- ❌ Migration log and audit trail

## 🎯 RECOMMENDED NEXT STEPS (Easiest to Hardest)

1. **Migration Logging** - Add logging to track what changes are being made during migration
2. **Dry-Run Mode** - Add a flag to simulate migrations without making changes
3. **Progress Reporting** - Add progress indicators for large dataset migrations
4. **Constraint Validation** - Add validation for NOT NULL and UNIQUE constraints before migration
5. **Index Preservation** - Preserve and recreate indexes during migration
6. **Foreign Key Validation** - Validate foreign key relationships before applying constraints

## 📝 NOTES

- The current implementation focuses on **safety first** - preventing data loss and ensuring atomic operations
- Schema versioning provides a foundation for more advanced features
- File locking ensures thread safety for concurrent access scenarios
- The migration approach (backup → new DB → migrate data → atomic replace) is robust and safe 

# from jes

 - ✅ we don't handle "query params" in the db name, mattn/go-sqlite3 let's you sql.Open("sqlite3", "foo.db?_busy_timeout=1000"), we should keep hold of the query string but treat only "foo.db" as the filename - **RESOLVED**
 - document that we mis-handle column/table renames
 - os.Remove(lockPath) is called unconditionally after unlock; consider deferring only on successful lock acquisition to avoid removing another process's lock file in some edge cases.
 - a ":memory:" database is apparently meant to be shared? so doesn't create a new db, need to make a temporary one on disk
 - make temporary files with better filenames maybe