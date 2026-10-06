# Migrations — read this before using them

**The historical files are not how the MySQL schema is created or upgraded.**
They look like golang-migrate migrations, and `internal/runtime` does load
them, but `000001`–`000014` are not the path a MySQL database takes. Running
those from zero fails. Migrations `000015` onward are active, ordered
transitions for databases stamped at the historical version.

This was verified against a real MySQL 8.0.46 instance rather than inferred:

- Applying the migrations to an empty MySQL database fails immediately on
  `000001_init_schema.up.sql`, which uses SQLite syntax
  (`AUTOINCREMENT`, 72 occurrences). MySQL wants `AUTO_INCREMENT`.
- Applying the historical `000002`–`000014` range to a database that was
initialised by the service fails on `ALTER TABLE agent_runs ADD COLUMN
dedupe_key ...`: those columns already exist, because AutoMigrate created them
from the Go structs. The run left `schema_migrations` marked `dirty`, which
then needs manual repair.

## How the schema is actually managed

`internal/runtime/migrations.go` has two branches:

| Database state | What runs | SQL files used |
| --- | --- | --- |
| No `users` table (fresh) | `AutoMigrate(models.AllModels())` + `alignMySQLPlatformSchema()` + `Force(21)` | **none** |
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
- **Do not run golang-migrate from zero against a MySQL deployment.** Use the
service migration path, which bootstraps fresh databases with AutoMigrate and
then advances existing, versioned databases through `000015` onward.
- **`000001_init_schema.*` is SQLite dialect** and cannot work on MySQL at all.
- **`000002`–`000014` describe schema that the structs already contain.** They
are historical, not pending work. **`000015` onward must stay runnable
up/down/up on MySQL** because existing deployments use them.
- The single-version stamp (`currentSchemaVersion = 21`) means a fresh install
  claims to be fully migrated. When you add a migration, you must bump both
  that constant and the assertion in `migrations_test.go`.

## Adding foreign keys

There are none today: `FOREIGN KEY` appears zero times across the migration
files, so referential integrity rests entirely on application code. Adding
constraints is worth doing, but not blindly - a constraint applied to a table
that already contains orphans fails, and on a large table it fails slowly.

`check-orphans.sql` in this directory reports the violations for the ten
relationships most worth constraining. It is read-only:

```bash
mysql -h <host> -u <user> -p <database> < backend/migrations/check-orphans.sql
```

All zeros means the constraints can be added. A non-zero count means the row
set needs a decision first - delete the orphan, repair its reference, or leave
that constraint out - and that decision is a product question, not a migration
one.

The check was run against a real MySQL 8.0.46 database: it passes on a freshly
bootstrapped schema, and inserting a single `conversation_members` row with a
non-existent `conversation_id` and `user_id` makes it report `1` for both
relationships.

Constraints, once added, belong in the structs (`internal/models/`) with GORM
`constraint` tags, not in these SQL files - see the section above for why.

## Why this file exists

Two sources of truth for one schema is a trap, and this one is silent: the
service starts fine, tests pass, and the failure only appears when someone
tries to use the migrations the way their name suggests. Writing it down is
cheaper than the next person rediscovering it.
