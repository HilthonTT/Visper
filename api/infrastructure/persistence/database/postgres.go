package database

import (
	"log"

	"github.com/hilthontt/visper/api/infrastructure/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var dbClient *gorm.DB

func InitDb(cfg *config.Config) error {
	// Timestamps are stored as TIMESTAMP WITH TIME ZONE; use UTC for the session.
	dsn := cfg.GetPostgresConnectionString() + " TimeZone=UTC"

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	if err := sqlDB.Ping(); err != nil {
		return err
	}

	sqlDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
	// ConnMaxLifetime is already a time.Duration parsed from the config ("5s").
	sqlDB.SetConnMaxLifetime(cfg.Postgres.ConnMaxLifetime)

	dbClient = db

	log.Println("Db connection established")
	return nil
}

func GetDb() *gorm.DB {
	return dbClient
}

func CloseDb() {
	if dbClient == nil {
		return
	}

	if con, err := dbClient.DB(); err == nil {
		_ = con.Close()
	}
}
