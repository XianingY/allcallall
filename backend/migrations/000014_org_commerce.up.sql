-- Phase 2: B2B organization billing, invoices, quota policies.
-- IF NOT EXISTS keeps this idempotent with the AutoMigrate bootstrap path.

CREATE TABLE IF NOT EXISTS organization_plans (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organization_id BIGINT UNSIGNED NOT NULL,
    plan_id VARCHAR(64) NOT NULL,
    plan_name VARCHAR(120),
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    billing_cycle VARCHAR(16) NOT NULL DEFAULT 'monthly',
    current_period_start DATETIME,
    current_period_end DATETIME,
    seats INT NOT NULL DEFAULT 1,
    created_at DATETIME,
    updated_at DATETIME,
    PRIMARY KEY (id),
    UNIQUE KEY idx_org_plan_current (organization_id),
    KEY idx_org_plans_plan (plan_id),
    KEY idx_org_plans_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS organization_usage_ledgers (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organization_id BIGINT UNSIGNED NOT NULL,
    feature VARCHAR(64) NOT NULL,
    period_key VARCHAR(16) NOT NULL,
    units BIGINT NOT NULL DEFAULT 0,
    limit_units BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME,
    PRIMARY KEY (id),
    UNIQUE KEY idx_org_usage_period (organization_id, feature, period_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS invoices (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organization_id BIGINT UNSIGNED NOT NULL,
    invoice_no VARCHAR(64) NOT NULL,
    plan_id VARCHAR(64),
    billing_period_start DATETIME,
    billing_period_end DATETIME,
    currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    subtotal_minor BIGINT NOT NULL DEFAULT 0,
    tax_minor BIGINT NOT NULL DEFAULT 0,
    total_minor BIGINT NOT NULL DEFAULT 0,
    tax_rate_permille INT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    issued_at DATETIME,
    created_at DATETIME,
    updated_at DATETIME,
    PRIMARY KEY (id),
    UNIQUE KEY uq_invoices_no (invoice_no),
    KEY idx_invoices_org (organization_id),
    KEY idx_invoices_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS invoice_lines (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    invoice_id BIGINT UNSIGNED NOT NULL,
    description TEXT,
    quantity BIGINT NOT NULL DEFAULT 1,
    unit_minor BIGINT NOT NULL DEFAULT 0,
    amount_minor BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_invoice_lines_invoice (invoice_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS quota_policies (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    plan_id VARCHAR(64) NOT NULL,
    feature VARCHAR(64) NOT NULL,
    limit_units BIGINT NOT NULL DEFAULT 0,
    unlimited TINYINT(1) NOT NULL DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME,
    PRIMARY KEY (id),
    UNIQUE KEY idx_quota_plan_feature (plan_id, feature)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
