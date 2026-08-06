package runtime

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	sqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

const currentSchemaVersion = 13

// RunMigrations applies the ordered schema migrations using golang-migrate.
func RunMigrations(db *gorm.DB, dataSourceNames ...string) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	dialectName := db.Dialector.Name()
	var migrationDB *sql.DB
	if dialectName == "mysql" && len(dataSourceNames) > 0 && dataSourceNames[0] != "" {
		config, parseErr := drivermysql.ParseDSN(dataSourceNames[0])
		if parseErr != nil {
			return fmt.Errorf("parse mysql migration DSN: %w", parseErr)
		}
		config.MultiStatements = true
		migrationDB, err = sql.Open("mysql", config.FormatDSN())
		if err != nil {
			return fmt.Errorf("open mysql migration connection: %w", err)
		}
		defer migrationDB.Close()
		sqlDB = migrationDB
	}
	var driver database.Driver
	if dialectName == "mysql" {
		driver, err = migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
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
	if dialectName == "mysql" {
		bootstrapped, err := bootstrapMySQLSchema(db, m)
		if err != nil {
			return err
		}
		if bootstrapped {
			log.Println("MySQL schema bootstrapped at current version.")
			return nil
		}
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

func bootstrapMySQLSchema(db *gorm.DB, migration *migrate.Migrate) (bool, error) {
	if db.Migrator().HasTable(&models.User{}) {
		_, _, err := migration.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			if err := migration.Force(1); err != nil {
				return false, fmt.Errorf("mark existing MySQL schema at version 1: %w", err)
			}
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("read MySQL migration version: %w", err)
		}
		return false, nil
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		return false, fmt.Errorf("bootstrap empty MySQL schema: %w", err)
	}
	if err := alignMySQLPlatformSchema(db); err != nil {
		return false, err
	}
	if err := migration.Force(currentSchemaVersion); err != nil {
		return false, fmt.Errorf("mark bootstrapped MySQL schema at version %d: %w", currentSchemaVersion, err)
	}
	return true, nil
}

func alignMySQLPlatformSchema(db *gorm.DB) error {
	statements := []string{
		"ALTER TABLE mcp_tools MODIFY COLUMN namespaced_name VARCHAR(255) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE mcp_executions MODIFY COLUMN execution_id VARCHAR(96) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE mcp_executions MODIFY COLUMN run_ref VARCHAR(96) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE mcp_executions MODIFY COLUMN tool_call_id VARCHAR(96) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE mcp_executions MODIFY COLUMN sandbox_request_digest CHAR(64) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE mcp_executions MODIFY COLUMN reconcile_attempts INT NOT NULL DEFAULT 0",
		"ALTER TABLE mcp_executions MODIFY COLUMN next_reconcile_at DATETIME(6) NULL",
		"ALTER TABLE sandbox_execution_receipts MODIFY COLUMN execution_id VARCHAR(96) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE sandbox_execution_receipts MODIFY COLUMN request_digest CHAR(64) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE sandbox_execution_receipts MODIFY COLUMN run_ref VARCHAR(96) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE sandbox_execution_receipts MODIFY COLUMN tool_call_id VARCHAR(96) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE sandbox_execution_receipts MODIFY COLUMN status VARCHAR(32) CHARACTER SET ascii NOT NULL",
		"ALTER TABLE sandbox_execution_receipts MODIFY COLUMN error_code VARCHAR(64) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE workflow_runs MODIFY COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE tool_approvals MODIFY COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE agent_runs MODIFY COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE agent_tool_calls MODIFY COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE workflow_runs MODIFY COLUMN execution_lease_token VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE agent_runs MODIFY COLUMN execution_lease_token VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT ''",
		"ALTER TABLE workflow_runs MODIFY COLUMN runtime_owner VARCHAR(32) CHARACTER SET ascii NOT NULL DEFAULT 'legacy_go'",
		"ALTER TABLE agent_runs MODIFY COLUMN runtime_owner VARCHAR(32) CHARACTER SET ascii NOT NULL DEFAULT 'legacy_go'",
		"ALTER TABLE workflow_runs MODIFY COLUMN runtime_request_json LONGTEXT NOT NULL",
		"ALTER TABLE agent_runs MODIFY COLUMN runtime_request_json LONGTEXT NOT NULL",
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("align empty MySQL platform schema: %w", err)
		}
	}
	return nil
}
