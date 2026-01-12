package main

import (
	"github.com/imlargo/medusa/internal/config"
	"github.com/imlargo/medusa/internal/database"
)

func main() {
	cfg := config.LoadConfig()

	// Database
	db, err := database.NewPostgresDatabase(cfg.Database.URL)
	if err != nil {
		panic("Could not connect to the database: " + err.Error())
		return
	}

	database.Migrate(db)
}
