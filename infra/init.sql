-- AllCallAll MySQL initialization (optional)
-- NOTE: MySQL official image only executes files in /docker-entrypoint-initdb.d
-- on FIRST initialization of the data directory (i.e. when mysql_data is empty).
--
-- This script is intentionally idempotent and does NOT set/alter passwords.

CREATE DATABASE IF NOT EXISTS allcallall_db
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

-- The docker-compose.yml sets MYSQL_USER=allcallall.
-- Ensure that user has privileges on the application database.
GRANT ALL PRIVILEGES ON allcallall_db.* TO 'allcallall'@'%';

FLUSH PRIVILEGES;
