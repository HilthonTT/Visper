package migration

import (
	"log"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/persistence/database"
	"gorm.io/gorm"
)

func Up1() {
	database := database.GetDb()
	createTables(database)
}

func createTables(database *gorm.DB) {
	tables := []any{}

	tables = addNewTable(database, model.AuditLog{}, tables)

	err := database.Migrator().CreateTable(tables...)
	if err != nil {
		log.Printf("Error migrating: %w\n", err)
	}
	log.Println("Tables Created")
}

func addNewTable(database *gorm.DB, model any, tables []any) []any {
	if !database.Migrator().HasTable(model) {
		tables = append(tables, model)
	}
	return tables
}
