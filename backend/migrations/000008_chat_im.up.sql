-- 即时通讯群聊领域（群组 / 群成员 / 消息漫游 / 已读回执）
CREATE TABLE IF NOT EXISTS chat_groups (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organization_id BIGINT UNSIGNED NOT NULL,
    kind VARCHAR(16) NOT NULL DEFAULT 'group',
    name VARCHAR(180) NOT NULL DEFAULT '',
    description VARCHAR(500) NOT NULL DEFAULT '',
    avatar_url VARCHAR(512) NOT NULL DEFAULT '',
    created_by BIGINT UNSIGNED NOT NULL,
    created_at TIMESTAMP NULL DEFAULT NULL,
    updated_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id),
    INDEX idx_chat_groups_org (organization_id),
    INDEX idx_chat_groups_kind (kind),
    INDEX idx_chat_groups_created_by (created_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS chat_group_members (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organization_id BIGINT UNSIGNED NOT NULL,
    group_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'member',
    muted_at TIMESTAMP NULL DEFAULT NULL,
    last_read_message_id BIGINT UNSIGNED NULL DEFAULT NULL,
    last_read_at TIMESTAMP NULL DEFAULT NULL,
    joined_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NULL DEFAULT NULL,
    updated_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE INDEX idx_chat_group_member (group_id, user_id),
    INDEX idx_chat_group_members_org (organization_id),
    INDEX idx_chat_group_members_user (user_id),
    INDEX idx_chat_group_members_last_read (last_read_message_id),
    INDEX idx_chat_group_members_last_read_at (last_read_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS chat_messages (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organization_id BIGINT UNSIGNED NOT NULL,
    group_id BIGINT UNSIGNED NOT NULL,
    sender_id BIGINT UNSIGNED NOT NULL,
    type VARCHAR(16) NOT NULL DEFAULT 'text',
    body LONGTEXT,
    metadata_json LONGTEXT,
    reply_to_id BIGINT UNSIGNED NULL DEFAULT NULL,
    edited_at TIMESTAMP NULL DEFAULT NULL,
    deleted_at TIMESTAMP NULL DEFAULT NULL,
    deleted_by BIGINT UNSIGNED NULL DEFAULT NULL,
    created_at TIMESTAMP NULL DEFAULT NULL,
    updated_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id),
    INDEX idx_chat_messages_org (organization_id),
    INDEX idx_chat_messages_group (group_id),
    INDEX idx_chat_messages_sender (sender_id),
    INDEX idx_chat_messages_reply_to (reply_to_id),
    INDEX idx_chat_message_group_created (group_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS chat_message_receipts (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organization_id BIGINT UNSIGNED NOT NULL,
    group_id BIGINT UNSIGNED NOT NULL,
    message_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    read_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE INDEX idx_chat_receipt (message_id, user_id),
    INDEX idx_chat_message_receipts_org (organization_id),
    INDEX idx_chat_message_receipts_group (group_id),
    INDEX idx_chat_message_receipts_user (user_id),
    INDEX idx_chat_message_receipts_read_at (read_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
