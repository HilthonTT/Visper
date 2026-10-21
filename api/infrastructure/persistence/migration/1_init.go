package migration

import (
	"fmt"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/persistence/database"
)

// Up1 creates or updates the tables (and their indexes) required by the API.
// AutoMigrate is idempotent: it creates missing tables, columns and indexes
// without touching existing data.
func Up1() error {
	db := database.GetDb()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		return fmt.Errorf("migrating tables: %w", err)
	}

	return nil
}
