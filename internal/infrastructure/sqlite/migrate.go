package sqlite

import (
	"fmt"

	"gorm.io/gorm"
)

// stripLegacyForeignKeys removes FOREIGN KEY constraints from legacy tables
// before GORM AutoMigrate runs.
//
// The old schema.sql declared `FOREIGN KEY ... REFERENCES ... ON DELETE
// CASCADE` on host_keys and sessions. glebarez's migrator mis-parses such
// constraints when it rebuilds a table (it treats the FOREIGN keyword as a
// column name), so AutoMigrate fails on pre-existing databases. SQLite has no
// ALTER TABLE ... DROP CONSTRAINT, so we rebuild each affected table without
// the constraint. Our application-level delete paths clean up child rows, so
// the database-level cascades are not load-bearing.
func stripLegacyForeignKeys(db *gorm.DB) error {
	affected := []string{"host_keys", "sessions"}
	for _, table := range affected {
		hasFK, err := tableHasForeignKey(db, table)
		if err != nil {
			return err
		}
		if !hasFK {
			continue
		}
		if err := rebuildTableWithoutFK(db, table); err != nil {
			return fmt.Errorf("rebuild %s without fk: %w", table, err)
		}
	}
	return nil
}

// tableHasForeignKey checks PRAGMA foreign_key_list for the table.
func tableHasForeignKey(db *gorm.DB, table string) (bool, error) {
	rows, err := db.Raw(fmt.Sprintf("PRAGMA foreign_key_list(%q)", table)).Rows()
	if err != nil {
		return false, err
	}
	defer rows.Close()
	// A row exists iff the table declares at least one foreign key.
	return rows.Next(), rows.Err()
}

// rebuildTableWithoutFK recreates table (keeping its rows) without the FOREIGN
// KEY clause. SQLite requires a rename + create + copy dance since there is no
// DROP CONSTRAINT. Foreign keys are disabled during the copy so the old
// references don't block it.
func rebuildTableWithoutFK(db *gorm.DB, table string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		old := table + "__old"
		if err := tx.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		// 1. Rename the legacy table out of the way.
		if err := tx.Exec(fmt.Sprintf("ALTER TABLE %q RENAME TO %q", table, old)).Error; err != nil {
			return err
		}
		// 2. Create the new (FK-free) table from the GORM model.
		model := modelForTable(table)
		if model == nil {
			return fmt.Errorf("no model for table %s", table)
		}
		if err := tx.Migrator().CreateTable(model); err != nil {
			return err
		}
		// 3. Copy rows.
		if err := tx.Exec(fmt.Sprintf("INSERT INTO %q SELECT * FROM %q", table, old)).Error; err != nil {
			return err
		}
		// 4. Drop the old table.
		if err := tx.Exec(fmt.Sprintf("DROP TABLE %q", old)).Error; err != nil {
			return err
		}
		return tx.Exec("PRAGMA foreign_keys = ON").Error
	})
}

// modelForTable maps a legacy table name to its GORM model (used to rebuild).
func modelForTable(table string) any {
	switch table {
	case "host_keys":
		return &hostKeyModel{}
	case "sessions":
		return &sessionModel{}
	}
	return nil
}
