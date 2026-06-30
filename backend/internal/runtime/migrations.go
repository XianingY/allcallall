package runtime

import (
	"errors"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/gorm"
)

// RunMigrations applies the ordered schema migrations using golang-migrate.
func RunMigrations(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	dialectName := db.Dialector.Name()
	var driver database.Driver
	if dialectName == "mysql" {
		driver, err = mysql.WithInstance(sqlDB, &mysql.Config{})
		if err != nil {
			return fmt.Errorf("failed to create mysql driver: %w", err)
		}
	} else if dialectName == "sqlite" {
		driver, err = sqlite.WithInstance(sqlDB, &sqlite.Config{})
		if err != nil {
			return fmt.Errorf("failed to create sqlite driver: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported database dialect: %s", dialectName)
	}

	// We assume migrations are stored in a relative 'migrations' directory
	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		dialectName, driver)
	if err != nil {
		return fmt.Errorf("failed to init migrate instance: %w", err)
	}

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	if errors.Is(err, migrate.ErrNoChange) {
		log.Println("No new migrations to apply.")
	} else {
		log.Println("Migrations applied successfully.")
	}

	return nil
}
