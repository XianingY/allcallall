package models

import "time"

const (
	RAGSourceKindManualText = "manual_text"
	RAGSourceKindURL        = "url"
	RAGSourceKindFile       = "file"

	RAGSourceStatusPending = "pending"
	RAGSourceStatusReady   = "ready"
	RAGSourceStatusFailed  = "failed"

	RAGSourceVersionStatusPending    = "pending"
	RAGSourceVersionStatusActive     = "active"
	RAGSourceVersionStatusSuperseded = "superseded"
	RAGSourceVersionStatusFailed     = "failed"

	RAGChunkIndexStatusPending = "pending"
	RAGChunkIndexStatusIndexed = "indexed"
	RAGChunkIndexStatusSkipped = "skipped"
	RAGChunkIndexStatusFailed  = "failed"

	RAGRetrievalModeVector      = "vector"
	RAGRetrievalModeSQLFallback = "sql_fallback"
)

// RAGSource stores an organization-scoped knowledge source, optionally bound to a conversation.
type RAGSource struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID  uint64    `gorm:"not null;index"`
	ConversationID  *uint64   `gorm:"index"`
	CreatedBy       uint64    `gorm:"not null;index"`
	Kind            string    `gorm:"size:32;not null;index"`
	Title           string    `gorm:"size:240;not null"`
	URI             string    `gorm:"size:1024"`
	FileName        string    `gorm:"size:255"`
	ContentType     string    `gorm:"size:120"`
	Status          string    `gorm:"size:32;not null;default:'pending';index"`
	ActiveVersionID *uint64   `gorm:"index"`
	LastError       string    `gorm:"type:text"`
	CreatedAt       time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

func (RAGSource) TableName() string {
	return "rag_sources"
}

// RAGSourceVersion stores the normalized source text and version identity used for chunking.
type RAGSourceVersion struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index"`
	SourceID       uint64     `gorm:"not null;index;uniqueIndex:idx_rag_source_version"`
	Version        int        `gorm:"not null;uniqueIndex:idx_rag_source_version"`
	ContentHash    string     `gorm:"size:64;not null;index"`
	RawText        string     `gorm:"type:longtext"`
	Status         string     `gorm:"size:32;not null;default:'pending';index"`
	ChunkCount     int        `gorm:"not null;default:0"`
	LastError      string     `gorm:"type:text"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
	ActivatedAt    *time.Time `gorm:"index"`
}

func (RAGSourceVersion) TableName() string {
	return "rag_source_versions"
}

// RAGChunk stores chunked knowledge content and its ES indexing status.
type RAGChunk struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID  uint64     `gorm:"not null;index"`
	ConversationID  *uint64    `gorm:"index"`
	SourceID        uint64     `gorm:"not null;index"`
	SourceVersionID uint64     `gorm:"not null;index;uniqueIndex:idx_rag_chunk_version_hash"`
	ChunkIndex      int        `gorm:"not null;index"`
	StartOffset     int        `gorm:"not null;default:0"`
	EndOffset       int        `gorm:"not null;default:0"`
	ContentHash     string     `gorm:"size:64;not null;uniqueIndex:idx_rag_chunk_version_hash;index"`
	Content         string     `gorm:"type:longtext;not null"`
	Keywords        string     `gorm:"type:text"`
	IndexStatus     string     `gorm:"size:32;not null;default:'pending';index"`
	LastError       string     `gorm:"type:text"`
	IndexedAt       *time.Time `gorm:"index"`
	CreatedAt       time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime"`
}

func (RAGChunk) TableName() string {
	return "rag_chunks"
}
