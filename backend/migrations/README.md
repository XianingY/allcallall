# Migrations — read this before using them

**These files are not how the MySQL schema is created or upgraded.** They look
like golang-migrate migrations, and `internal/runtime` does load them, but on
MySQL they are not the path anything actually takes. Running them against a
MySQL database fails.

This was verified against a real MySQL 8.0.46 instance rather than inferred:

- Applying the migrations to an empty MySQL database fails immediately on
  `000001_init_schema.up.sql`, which uses SQLite syntax
  (`AUTOINCREMENT`, 72 occurrences). MySQL wants `AUTO_INCREMENT`.
- Applying `000002` onward to a database that was initialised by the service
  fails on `ALTER TABLE agent_runs ADD COLUMN dedupe_key ...`: those columns
  already exist, because AutoMigrate created them from the Go structs. The run
  left `schema_migrations` marked `dirty`, which then needs manual repair.

## How the schema is actually managed

`internal/runtime/migrations.go` has two branches:

| Database state | What runs | SQL files used |
| --- | --- | --- |
| No `users` table (fresh) | `AutoMigrate(models.AllModels())` + `alignMySQLPlatformSchema()` + `Force(14)` | **none** |
| `users` table present | `migration.Up()` | any migration above the recorded version |

So on a fresh install the SQL files are skipped entirely and the version is
stamped at the latest one. The Go structs in `internal/models/` are the
authority for the MySQL schema; `alignMySQLPlatformSchema` in
`internal/runtime/migrations.go` patches the handful of places where
AutoMigrate's generated types differ from what the platform expects (character
sets, column widths).

## What this means in practice

- **Adding a column or table for MySQL**: change the struct in
  `internal/models/`. AutoMigrate applies it on the next start. There is no SQL
  file to write.
- **Do not run `migrate` against a MySQL deployment.** It will fail, or worse,
  half-apply and leave the database dirty.
- **`000001_init_schema.*` is SQLite dialect** and cannot work on MySQL at all.
- **`000002`–`000014` describe schema that the structs already contain.** They
  are historical, not pending work.
- The single-version stamp (`currentSchemaVersion = 14`) means a fresh install
  claims to be fully migrated. When you add a migration, you must bump both
  that constant and the assertion in `migrations_test.go`.

## Why this file exists

Two sources of truth for one schema is a trap, and this one is silent: the
service starts fine, tests pass, and the failure only appears when someone
tries to use the migrations the way their name suggests. Writing it down is
cheaper than the next person rediscovering it.
