package runtime

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/google/uuid"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCurrentSchemaVersionIncludesDurableSandboxReceipts(t *testing.T) {
	if currentSchemaVersion != 8 {
		t.Fatalf("current schema version=%d want=8", currentSchemaVersion)
	}
	migrations := map[string]map[string][]string{
		"000003_workflow_runtime_resume": {
			"up":   {"approval_request_id", "approval_checkpoint_version", "runtime_request_json", "runtime_owner"},
			"down": {"approval_request_id", "approval_checkpoint_version", "runtime_request_json", "runtime_owner"},
		},
		"000004_sandbox_execution_receipts": {
			"up":   {"sandbox_execution_receipts", "request_digest", "reconcile_attempts", "next_reconcile_at", "idx_mcp_executions_reconcile"},
			"down": {"sandbox_execution_receipts", "reconcile_attempts", "next_reconcile_at", "idx_mcp_executions_reconcile"},
		},
	}
	for migration, directions := range migrations {
		for direction, fields := range directions {
			path := filepath.Join("..", "..", "migrations", migration+"."+direction+".sql")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s migration: %v", direction, err)
			}
			text := string(raw)
			for _, field := range fields {
				if !strings.Contains(text, field) {
					t.Fatalf("%s migration missing %s", direction, field)
				}
			}
		}
	}
}

func TestMySQLWorkflowRuntimeMigrationUpDownUp(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ALLCALLALL_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("ALLCALLALL_TEST_MYSQL_DSN is not configured")
	}

	databaseName := "allcallall_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	adminDB, testDSN := createMigrationTestDatabase(t, dsn, databaseName)
	t.Cleanup(func() {
		if _, err := adminDB.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`"); err != nil {
			t.Errorf("drop isolated migration database: %v", err)
		}
	})

	gormDB, err := gorm.Open(gormmysql.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open isolated migration database with gorm: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get isolated migration sql.DB: %v", err)
	}

	driver, err := migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
	if err != nil {
		t.Fatalf("create MySQL migration driver: %v", err)
	}
	migrationPath, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migration directory: %v", err)
	}
	migration, err := migrate.NewWithDatabaseInstance(
		"file://"+filepath.ToSlash(migrationPath),
		"mysql",
		driver,
	)
	if err != nil {
		t.Fatalf("create MySQL migration runner: %v", err)
	}
	t.Cleanup(func() {
		sourceErr, databaseErr := migration.Close()
		if sourceErr != nil {
			t.Errorf("close migration source: %v", sourceErr)
		}
		if databaseErr != nil {
			t.Errorf("close migration database: %v", databaseErr)
		}
	})

	// MySQL's supported initial-schema path uses AutoMigrate because migration 000001
	// is the historical SQLite export. Move that isolated schema back to v2 before
	// seeding so every v3 transition below executes the real ordered SQL migration.
	bootstrapped, err := bootstrapMySQLSchema(gormDB, migration)
	if err != nil {
		t.Fatalf("bootstrap isolated MySQL schema: %v", err)
	}
	if !bootstrapped {
		t.Fatal("expected empty isolated database to be bootstrapped")
	}
	assertMigrationVersion(t, migration, 8)
	assertSandboxReceiptV4Schema(t, sqlDB, databaseName)
	assertWorkflowRuntimeV3Schema(t, sqlDB, databaseName)
	if err := migration.Migrate(2); err != nil {
		t.Fatalf("move bootstrapped schema to v2: %v", err)
	}
	assertMigrationVersion(t, migration, 2)
	assertWorkflowRuntimeV3SchemaAbsent(t, sqlDB, databaseName)

	seedWorkflowRuntimeV2Rows(t, sqlDB)

	if err := migration.Migrate(3); err != nil {
		t.Fatalf("migrate workflow runtime schema v2 to v3: %v", err)
	}
	assertMigrationVersion(t, migration, 3)
	assertWorkflowRuntimeV3Schema(t, sqlDB, databaseName)
	assertWorkflowRuntimeBackfill(t, sqlDB)
	setWorkflowRuntimeV3ApprovalData(t, sqlDB)

	if err := migration.Migrate(2); err != nil {
		t.Fatalf("roll back workflow runtime schema v3 to v2: %v", err)
	}
	assertMigrationVersion(t, migration, 2)
	assertWorkflowRuntimeV3SchemaAbsent(t, sqlDB, databaseName)
	assertWorkflowRuntimeV2DataPreserved(t, sqlDB)

	if err := migration.Migrate(3); err != nil {
		t.Fatalf("reapply workflow runtime schema v2 to v3: %v", err)
	}
	assertMigrationVersion(t, migration, 3)
	assertWorkflowRuntimeV3Schema(t, sqlDB, databaseName)
	assertWorkflowRuntimeBackfill(t, sqlDB)
	assertWorkflowRuntimeV2DataPreserved(t, sqlDB)
}

func TestMySQLSandboxReceiptMigrationUpDownUp(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ALLCALLALL_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("ALLCALLALL_TEST_MYSQL_DSN is not configured")
	}

	databaseName := "allcallall_sandbox_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	adminDB, testDSN := createMigrationTestDatabase(t, dsn, databaseName)
	t.Cleanup(func() {
		if _, err := adminDB.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`"); err != nil {
			t.Errorf("drop isolated sandbox migration database: %v", err)
		}
	})

	gormDB, err := gorm.Open(gormmysql.Open(testDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open isolated sandbox migration database: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get isolated sandbox migration sql.DB: %v", err)
	}
	driver, err := migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
	if err != nil {
		t.Fatalf("create sandbox migration driver: %v", err)
	}
	migrationPath, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migration directory: %v", err)
	}
	migration, err := migrate.NewWithDatabaseInstance("file://"+filepath.ToSlash(migrationPath), "mysql", driver)
	if err != nil {
		t.Fatalf("create sandbox migration runner: %v", err)
	}
	t.Cleanup(func() {
		sourceErr, databaseErr := migration.Close()
		if sourceErr != nil {
			t.Errorf("close sandbox migration source: %v", sourceErr)
		}
		if databaseErr != nil {
			t.Errorf("close sandbox migration database: %v", databaseErr)
		}
	})

	bootstrapped, err := bootstrapMySQLSchema(gormDB, migration)
	if err != nil {
		t.Fatalf("bootstrap sandbox receipt schema: %v", err)
	}
	if !bootstrapped {
		t.Fatal("expected empty sandbox migration database to be bootstrapped")
	}
	assertMigrationVersion(t, migration, 8)
	assertSandboxReceiptV4Schema(t, sqlDB, databaseName)

	if err := migration.Migrate(3); err != nil {
		t.Fatalf("roll back sandbox receipt schema to v3: %v", err)
	}
	assertMigrationVersion(t, migration, 3)
	assertSandboxReceiptV4SchemaAbsent(t, sqlDB, databaseName)

	if err := migration.Migrate(4); err != nil {
		t.Fatalf("reapply sandbox receipt schema: %v", err)
	}
	assertMigrationVersion(t, migration, 4)
	assertSandboxReceiptV4Schema(t, sqlDB, databaseName)

	if err := migration.Migrate(3); err != nil {
		t.Fatalf("second sandbox receipt rollback: %v", err)
	}
	assertSandboxReceiptV4SchemaAbsent(t, sqlDB, databaseName)
	if err := migration.Migrate(4); err != nil {
		t.Fatalf("second sandbox receipt reapply: %v", err)
	}
	assertSandboxReceiptV4Schema(t, sqlDB, databaseName)
}

func createMigrationTestDatabase(t *testing.T, dsn, databaseName string) (*sql.DB, string) {
	t.Helper()

	parsed, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse ALLCALLALL_TEST_MYSQL_DSN: %v", err)
	}
	adminConfig := *parsed
	adminConfig.DBName = ""
	adminConfig.MultiStatements = false
	adminDB, err := sql.Open("mysql", adminConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL administrative connection: %v", err)
	}
	t.Cleanup(func() {
		if err := adminDB.Close(); err != nil {
			t.Errorf("close MySQL administrative connection: %v", err)
		}
	})
	if err := adminDB.Ping(); err != nil {
		t.Fatalf("ping MySQL administrative connection: %v", err)
	}
	if _, err := adminDB.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create isolated migration database: %v", err)
	}

	testConfig := *parsed
	testConfig.DBName = databaseName
	testConfig.MultiStatements = true
	return adminDB, testConfig.FormatDSN()
}

func assertMigrationVersion(t *testing.T, migration *migrate.Migrate, want uint) {
	t.Helper()
	version, dirty, err := migration.Version()
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != want || dirty {
		t.Fatalf("migration version=(%d, dirty=%t) want=(%d, dirty=false)", version, dirty, want)
	}
}

type mysqlColumnExpectation struct {
	table              string
	column             string
	dataType           string
	columnTypeContains string
	nullable           string
	defaultValue       *string
	characterSet       string
}

func assertWorkflowRuntimeV3Schema(t *testing.T, db *sql.DB, databaseName string) {
	t.Helper()
	empty := ""
	zero := "0"
	legacyOwner := "legacy_go"
	columns := []mysqlColumnExpectation{
		{table: "workflow_runs", column: "approval_request_id", dataType: "varchar", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "workflow_runs", column: "runtime_request_json", dataType: "longtext", nullable: "NO"},
		{table: "workflow_runs", column: "execution_lease_token", dataType: "varchar", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "workflow_runs", column: "runtime_owner", dataType: "varchar", nullable: "NO", defaultValue: &legacyOwner, characterSet: "ascii"},
		{table: "agent_runs", column: "approval_request_id", dataType: "varchar", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "agent_runs", column: "runtime_request_json", dataType: "longtext", nullable: "NO"},
		{table: "agent_runs", column: "execution_lease_token", dataType: "varchar", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "agent_runs", column: "runtime_owner", dataType: "varchar", nullable: "NO", defaultValue: &legacyOwner, characterSet: "ascii"},
		{table: "agent_tool_calls", column: "approval_request_id", dataType: "varchar", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "agent_tool_calls", column: "approval_checkpoint_version", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
		{table: "agent_tool_calls", column: "decision", dataType: "varchar", nullable: "NO", defaultValue: &empty},
		{table: "agent_tool_calls", column: "decided_by", dataType: "bigint", columnTypeContains: "unsigned", nullable: "YES"},
		{table: "agent_tool_calls", column: "decided_at", dataType: "datetime", columnTypeContains: "datetime(6)", nullable: "YES"},
		{table: "agent_tool_calls", column: "mcp_installation_id", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
		{table: "agent_tool_calls", column: "mcp_revision_id", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
		{table: "agent_tool_calls", column: "mcp_tool_id", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
		{table: "tool_approvals", column: "approval_request_id", dataType: "varchar", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "tool_approvals", column: "approval_checkpoint_version", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
		{table: "tool_approvals", column: "mcp_installation_id", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
		{table: "tool_approvals", column: "mcp_revision_id", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
		{table: "tool_approvals", column: "mcp_tool_id", dataType: "bigint", columnTypeContains: "unsigned", nullable: "NO", defaultValue: &zero},
	}
	for _, expected := range columns {
		assertMySQLColumn(t, db, databaseName, expected)
	}

	indexes := map[string]map[string]string{
		"workflow_runs": {
			"idx_workflow_runs_approval_request_id":   "approval_request_id",
			"idx_workflow_runs_execution_lease_token": "execution_lease_token",
			"idx_workflow_runs_runtime_owner":         "runtime_owner",
		},
		"agent_runs": {
			"idx_agent_runs_approval_request_id":   "approval_request_id",
			"idx_agent_runs_execution_lease_token": "execution_lease_token",
			"idx_agent_runs_runtime_owner":         "runtime_owner",
		},
		"agent_tool_calls": {
			"idx_agent_tool_calls_approval_request_id":         "approval_request_id",
			"idx_agent_tool_calls_approval_checkpoint_version": "approval_checkpoint_version",
			"idx_agent_tool_calls_decided_by":                  "decided_by",
			"idx_agent_tool_calls_decided_at":                  "decided_at",
			"idx_agent_tool_calls_mcp_installation_id":         "mcp_installation_id",
			"idx_agent_tool_calls_mcp_revision_id":             "mcp_revision_id",
			"idx_agent_tool_calls_mcp_tool_id":                 "mcp_tool_id",
		},
		"tool_approvals": {
			"idx_tool_approvals_approval_request_id":         "approval_request_id",
			"idx_tool_approvals_approval_checkpoint_version": "approval_checkpoint_version",
			"idx_tool_approvals_mcp_installation_id":         "mcp_installation_id",
			"idx_tool_approvals_mcp_revision_id":             "mcp_revision_id",
			"idx_tool_approvals_mcp_tool_id":                 "mcp_tool_id",
		},
	}
	for table, tableIndexes := range indexes {
		for index, column := range tableIndexes {
			assertMySQLIndex(t, db, databaseName, table, index, column)
		}
	}
}

func assertMySQLColumn(t *testing.T, db *sql.DB, databaseName string, expected mysqlColumnExpectation) {
	t.Helper()
	var dataType, columnType, nullable string
	var defaultValue, characterSet sql.NullString
	err := db.QueryRow(`
		SELECT DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, CHARACTER_SET_NAME
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		databaseName, expected.table, expected.column,
	).Scan(&dataType, &columnType, &nullable, &defaultValue, &characterSet)
	if err != nil {
		t.Fatalf("read %s.%s metadata: %v", expected.table, expected.column, err)
	}
	if dataType != expected.dataType || nullable != expected.nullable {
		t.Fatalf("%s.%s type/nullability=(%s %s, %s) want=(%s, %s)",
			expected.table, expected.column, dataType, columnType, nullable, expected.dataType, expected.nullable)
	}
	if expected.columnTypeContains != "" && !strings.Contains(columnType, expected.columnTypeContains) {
		t.Fatalf("%s.%s column type=%s missing %q", expected.table, expected.column, columnType, expected.columnTypeContains)
	}
	if expected.defaultValue == nil {
		if defaultValue.Valid {
			t.Fatalf("%s.%s default=%q want NULL", expected.table, expected.column, defaultValue.String)
		}
	} else if !defaultValue.Valid || defaultValue.String != *expected.defaultValue {
		t.Fatalf("%s.%s default=(%q, valid=%t) want=%q", expected.table, expected.column, defaultValue.String, defaultValue.Valid, *expected.defaultValue)
	}
	if expected.characterSet != "" && (!characterSet.Valid || characterSet.String != expected.characterSet) {
		t.Fatalf("%s.%s charset=(%q, valid=%t) want=%q", expected.table, expected.column, characterSet.String, characterSet.Valid, expected.characterSet)
	}
}

func assertMySQLIndex(t *testing.T, db *sql.DB, databaseName, table, index, wantColumn string) {
	t.Helper()
	var column string
	var nonUnique int
	err := db.QueryRow(`
		SELECT COLUMN_NAME, NON_UNIQUE
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ? AND SEQ_IN_INDEX = 1`,
		databaseName, table, index,
	).Scan(&column, &nonUnique)
	if err != nil {
		t.Fatalf("read index %s.%s: %v", table, index, err)
	}
	if column != wantColumn || nonUnique != 1 {
		t.Fatalf("index %s.%s=(column=%s, non_unique=%d) want=(column=%s, non_unique=1)", table, index, column, nonUnique, wantColumn)
	}
}

func assertSandboxReceiptV4Schema(t *testing.T, db *sql.DB, databaseName string) {
	t.Helper()
	empty := ""
	zero := "0"
	columns := []mysqlColumnExpectation{
		{table: "mcp_executions", column: "sandbox_request_digest", dataType: "char", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "mcp_executions", column: "reconcile_attempts", dataType: "int", nullable: "NO", defaultValue: &zero},
		{table: "mcp_executions", column: "next_reconcile_at", dataType: "datetime", columnTypeContains: "datetime(6)", nullable: "YES"},
		{table: "sandbox_execution_receipts", column: "execution_id", dataType: "varchar", nullable: "NO", characterSet: "ascii"},
		{table: "sandbox_execution_receipts", column: "request_digest", dataType: "char", nullable: "NO", characterSet: "ascii"},
		{table: "sandbox_execution_receipts", column: "run_ref", dataType: "varchar", nullable: "NO", characterSet: "ascii"},
		{table: "sandbox_execution_receipts", column: "tool_call_id", dataType: "varchar", nullable: "NO", characterSet: "ascii"},
		{table: "sandbox_execution_receipts", column: "status", dataType: "varchar", nullable: "NO", characterSet: "ascii"},
		{table: "sandbox_execution_receipts", column: "output_json", dataType: "longblob", nullable: "YES"},
		{table: "sandbox_execution_receipts", column: "error_code", dataType: "varchar", nullable: "NO", defaultValue: &empty, characterSet: "ascii"},
		{table: "sandbox_execution_receipts", column: "stale_at", dataType: "datetime", columnTypeContains: "datetime(6)", nullable: "NO"},
		{table: "sandbox_execution_receipts", column: "expires_at", dataType: "datetime", columnTypeContains: "datetime(6)", nullable: "NO"},
	}
	for _, expected := range columns {
		assertMySQLColumn(t, db, databaseName, expected)
	}
	assertMySQLIndex(t, db, databaseName, "sandbox_execution_receipts", "idx_sandbox_receipt_request_digest", "request_digest")
	assertMySQLIndex(t, db, databaseName, "sandbox_execution_receipts", "idx_sandbox_receipt_status_stale", "status")
	assertMySQLIndexColumns(t, db, databaseName, "mcp_executions", "idx_mcp_executions_reconcile", []string{"status", "next_reconcile_at", "id"})
}

func assertMySQLIndexColumns(t *testing.T, db *sql.DB, databaseName, table, index string, want []string) {
	t.Helper()
	rows, err := db.Query(`
		SELECT COLUMN_NAME
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?
		ORDER BY SEQ_IN_INDEX`, databaseName, table, index)
	if err != nil {
		t.Fatalf("read index %s.%s columns: %v", table, index, err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan index %s.%s column: %v", table, index, err)
		}
		got = append(got, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate index %s.%s columns: %v", table, index, err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("index %s.%s columns=%v want=%v", table, index, got, want)
	}
}

func assertSandboxReceiptV4SchemaAbsent(t *testing.T, db *sql.DB, databaseName string) {
	t.Helper()
	var tableCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'sandbox_execution_receipts'`, databaseName).Scan(&tableCount); err != nil {
		t.Fatalf("count rolled-back sandbox receipt table: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("sandbox_execution_receipts still exists after v4 rollback")
	}
	var indexCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.STATISTICS
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'mcp_executions' AND INDEX_NAME = 'idx_mcp_executions_reconcile'`,
		databaseName,
	).Scan(&indexCount); err != nil {
		t.Fatalf("count rolled-back MCP reconcile index: %v", err)
	}
	if indexCount != 0 {
		t.Fatal("MCP reconcile index still exists after v4 rollback")
	}
	var digestColumnCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'mcp_executions' AND COLUMN_NAME = 'sandbox_request_digest'`,
		databaseName,
	).Scan(&digestColumnCount); err != nil {
		t.Fatalf("count rolled-back MCP request digest column: %v", err)
	}
	if digestColumnCount != 0 {
		t.Fatal("MCP request digest column still exists after v4 rollback")
	}
	for _, column := range []string{"reconcile_attempts", "next_reconcile_at"} {
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'mcp_executions' AND COLUMN_NAME = ?`,
			databaseName, column,
		).Scan(&count); err != nil {
			t.Fatalf("count rolled-back MCP reconciliation column %s: %v", column, err)
		}
		if count != 0 {
			t.Fatalf("MCP reconciliation column %s still exists after v4 rollback", column)
		}
	}
}

func assertWorkflowRuntimeV3SchemaAbsent(t *testing.T, db *sql.DB, databaseName string) {
	t.Helper()
	columns := map[string][]string{
		"workflow_runs":    {"approval_request_id", "runtime_request_json", "execution_lease_token", "runtime_owner"},
		"agent_runs":       {"approval_request_id", "runtime_request_json", "execution_lease_token", "runtime_owner"},
		"agent_tool_calls": {"approval_request_id", "approval_checkpoint_version", "decision", "decided_by", "decided_at", "mcp_installation_id", "mcp_revision_id", "mcp_tool_id"},
		"tool_approvals":   {"approval_request_id", "approval_checkpoint_version", "mcp_installation_id", "mcp_revision_id", "mcp_tool_id"},
	}
	for table, tableColumns := range columns {
		for _, column := range tableColumns {
			var count int
			if err := db.QueryRow(`
				SELECT COUNT(*) FROM information_schema.COLUMNS
				WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
				databaseName, table, column,
			).Scan(&count); err != nil {
				t.Fatalf("count rolled-back column %s.%s: %v", table, column, err)
			}
			if count != 0 {
				t.Fatalf("column %s.%s still exists after v3 rollback", table, column)
			}
		}
	}

	indexNames := []string{
		"idx_workflow_runs_approval_request_id", "idx_workflow_runs_execution_lease_token", "idx_workflow_runs_runtime_owner",
		"idx_agent_runs_approval_request_id", "idx_agent_runs_execution_lease_token", "idx_agent_runs_runtime_owner",
		"idx_agent_tool_calls_approval_request_id", "idx_agent_tool_calls_approval_checkpoint_version",
		"idx_agent_tool_calls_decided_by", "idx_agent_tool_calls_decided_at",
		"idx_agent_tool_calls_mcp_installation_id", "idx_agent_tool_calls_mcp_revision_id", "idx_agent_tool_calls_mcp_tool_id",
		"idx_tool_approvals_approval_request_id", "idx_tool_approvals_approval_checkpoint_version",
		"idx_tool_approvals_mcp_installation_id", "idx_tool_approvals_mcp_revision_id", "idx_tool_approvals_mcp_tool_id",
	}
	for _, index := range indexNames {
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.STATISTICS
			WHERE TABLE_SCHEMA = ? AND INDEX_NAME = ?`, databaseName, index).Scan(&count); err != nil {
			t.Fatalf("count rolled-back index %s: %v", index, err)
		}
		if count != 0 {
			t.Fatalf("index %s still exists after v3 rollback", index)
		}
	}
}

func seedWorkflowRuntimeV2Rows(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO workflow_runs
			(id, organization_id, user_id, conversation_id, idempotency_key, request_id, status, workflow_type, workflow_version, state_json, goal, created_at, updated_at)
		 VALUES
			(1001, 1, 10, 20, 'workflow-langgraph', 'request-workflow-langgraph', 'requires_action', 'agent_lab', 'agent_lab_langgraph_v1', '{"phase":"approval"}', 'langgraph goal', NOW(6), NOW(6)),
			(1002, 1, 10, 21, 'workflow-legacy', 'request-workflow-legacy', 'queued', 'agent_lab', 'agent_lab_v1', '{"phase":"queued"}', 'legacy goal', NOW(6), NOW(6))`,
		`INSERT INTO agent_runs
			(id, organization_id, user_id, conversation_id, idempotency_key, request_id, source, role, status, goal, created_at, updated_at)
		 VALUES
			(2001, 1, 10, 20, 'agent-langgraph', 'request-agent-langgraph', 'python_langgraph', 'primary', 'requires_action', 'python goal', NOW(6), NOW(6)),
			(2002, 1, 10, 21, 'agent-legacy', 'request-agent-legacy', 'deterministic', 'primary', 'queued', 'legacy goal', NOW(6), NOW(6))`,
		`INSERT INTO agent_tool_calls
			(id, run_id, call_id, tool_name, status, tool_schema_version, input_json, output_json, error_message, created_at, updated_at)
		 VALUES
			(3001, 2001, 'agent-call-3001', 'mcp.7.lookup', 'pending', 'mcp.v1', '{"query":"alpha"}', '{"old":"agent-output"}', 'agent-error-marker', NOW(6), NOW(6))`,
		`INSERT INTO tool_approvals
			(id, workflow_run_id, task_id, organization_id, tool_call_id, tool_name, status, tool_schema_version, input_json, output_json, error_message, requested_by, requested_at, created_at, updated_at)
		 VALUES
			(4001, 1001, 5001, 1, 'workflow-call-4001', 'mcp.8.lookup', 'pending', 'mcp.v1', '{"query":"beta"}', '{"old":"workflow-output"}', 'workflow-error-marker', 10, NOW(6), NOW(6), NOW(6))`,
	}
	for i, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed v2 migration row %d: %v", i+1, err)
		}
	}
}

func assertWorkflowRuntimeBackfill(t *testing.T, db *sql.DB) {
	t.Helper()
	checks := []struct {
		query string
		args  []any
		want  string
	}{
		{query: "SELECT runtime_owner FROM workflow_runs WHERE id = ?", args: []any{1001}, want: "python_langgraph"},
		{query: "SELECT runtime_owner FROM workflow_runs WHERE id = ?", args: []any{1002}, want: "legacy_go"},
		{query: "SELECT runtime_owner FROM agent_runs WHERE id = ?", args: []any{2001}, want: "python_langgraph"},
		{query: "SELECT runtime_owner FROM agent_runs WHERE id = ?", args: []any{2002}, want: "legacy_go"},
		{query: "SELECT runtime_request_json FROM workflow_runs WHERE id = ?", args: []any{1001}, want: ""},
		{query: "SELECT runtime_request_json FROM agent_runs WHERE id = ?", args: []any{2001}, want: ""},
		{query: "SELECT approval_request_id FROM agent_tool_calls WHERE id = ?", args: []any{3001}, want: ""},
		{query: "SELECT approval_request_id FROM tool_approvals WHERE id = ?", args: []any{4001}, want: ""},
	}
	for _, check := range checks {
		var got string
		if err := db.QueryRow(check.query, check.args...).Scan(&got); err != nil {
			t.Fatalf("read v3 backfill: %v", err)
		}
		if got != check.want {
			t.Fatalf("v3 backfill=%q want=%q for %s", got, check.want, check.query)
		}
	}

	var agentCheckpointVersion, workflowCheckpointVersion uint64
	var agentInstallationID, agentRevisionID, agentToolID uint64
	var workflowInstallationID, workflowRevisionID, workflowToolID uint64
	if err := db.QueryRow(`
		SELECT approval_checkpoint_version, mcp_installation_id, mcp_revision_id, mcp_tool_id
		FROM agent_tool_calls WHERE id = 3001`).Scan(
		&agentCheckpointVersion, &agentInstallationID, &agentRevisionID, &agentToolID,
	); err != nil {
		t.Fatalf("read agent approval defaults: %v", err)
	}
	if err := db.QueryRow(`
		SELECT approval_checkpoint_version, mcp_installation_id, mcp_revision_id, mcp_tool_id
		FROM tool_approvals WHERE id = 4001`).Scan(
		&workflowCheckpointVersion, &workflowInstallationID, &workflowRevisionID, &workflowToolID,
	); err != nil {
		t.Fatalf("read workflow approval defaults: %v", err)
	}
	if agentCheckpointVersion != 0 || agentInstallationID != 0 || agentRevisionID != 0 || agentToolID != 0 ||
		workflowCheckpointVersion != 0 || workflowInstallationID != 0 || workflowRevisionID != 0 || workflowToolID != 0 {
		t.Fatalf("v3 approval numeric defaults are not zero: agent=(%d,%d,%d,%d) workflow=(%d,%d,%d,%d)",
			agentCheckpointVersion, agentInstallationID, agentRevisionID, agentToolID,
			workflowCheckpointVersion, workflowInstallationID, workflowRevisionID, workflowToolID)
	}
}

func setWorkflowRuntimeV3ApprovalData(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`UPDATE agent_tool_calls
		 SET approval_request_id = 'agent-approval-request', approval_checkpoint_version = 11,
		     decision = 'approved', decided_by = 10, decided_at = NOW(6),
		     mcp_installation_id = 7, mcp_revision_id = 71, mcp_tool_id = 711
		 WHERE id = 3001`,
		`UPDATE tool_approvals
		 SET approval_request_id = 'workflow-approval-request', approval_checkpoint_version = 12,
		     mcp_installation_id = 8, mcp_revision_id = 81, mcp_tool_id = 811
		 WHERE id = 4001`,
	}
	for i, statement := range statements {
		result, err := db.Exec(statement)
		if err != nil {
			t.Fatalf("set v3 approval data %d: %v", i+1, err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			t.Fatalf("read v3 approval update rows %d: %v", i+1, err)
		}
		if rows != 1 {
			t.Fatalf("v3 approval update %d affected %d rows want=1", i+1, rows)
		}
	}
}

func assertWorkflowRuntimeV2DataPreserved(t *testing.T, db *sql.DB) {
	t.Helper()
	checks := []struct {
		query string
		want  string
	}{
		{query: "SELECT state_json FROM workflow_runs WHERE id = 1001", want: `{"phase":"approval"}`},
		{query: "SELECT goal FROM agent_runs WHERE id = 2001", want: "python goal"},
		{query: "SELECT input_json FROM agent_tool_calls WHERE id = 3001", want: `{"query":"alpha"}`},
		{query: "SELECT output_json FROM agent_tool_calls WHERE id = 3001", want: `{"old":"agent-output"}`},
		{query: "SELECT error_message FROM agent_tool_calls WHERE id = 3001", want: "agent-error-marker"},
		{query: "SELECT input_json FROM tool_approvals WHERE id = 4001", want: `{"query":"beta"}`},
		{query: "SELECT output_json FROM tool_approvals WHERE id = 4001", want: `{"old":"workflow-output"}`},
		{query: "SELECT error_message FROM tool_approvals WHERE id = 4001", want: "workflow-error-marker"},
	}
	for _, check := range checks {
		var got string
		if err := db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("read data preserved by v3 rollback: %v", err)
		}
		if got != check.want {
			t.Fatalf("preserved value=%q want=%q for %s", got, check.want, check.query)
		}
	}
}
