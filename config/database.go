package config

import (
	"roster-swap-api/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(AppConfig.DBPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.UserPreference{},
		&models.ApproverDelegate{},
		&models.Shift{},
		&models.SwapRequest{},
		&models.OperationLog{},
		&models.Notification{},
	)
	if err != nil {
		return nil, err
	}

	DB = db
	return db, nil
}
