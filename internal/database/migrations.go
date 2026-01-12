package database

import (
	"github.com/imlargo/medusa/internal/models"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {

	err := db.AutoMigrate(
		&models.User{},
	)

	return err
}
