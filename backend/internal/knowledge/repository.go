package knowledge

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// Repository encapsulates database operations for knowledge sources, chunks, and duplicates.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new knowledge repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ---------- Organization / Conversation Access ----------

// CountOrganizationMembers returns the number of members in an organization.
func (r *Repository) CountOrganizationMembers(ctx context.Context, organizationID, userID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		Count(&count).Error
	return count, err
}

// GetOrganizationMember returns a member record if it exists.
func (r *Repository) GetOrganizationMember(ctx context.Context, organizationID, userID uint64) (*models.OrganizationMember, error) {
	var member models.OrganizationMember
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		Take(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// CountConversationMembers returns the number of members in a conversation.
func (r *Repository) CountConversationMembers(ctx context.Context, organizationID, userID, conversationID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversations.id = ? AND conversation_members.user_id = ?", organizationID, conversationID, userID).
		Count(&count).Error
	return count, err
}

// ---------- RAGSource ----------

// CreateSource persists a new RAGSource.
func (r *Repository) CreateSource(ctx context.Context, source *models.RAGSource) error {
	return r.db.WithContext(ctx).Create(source).Error
}

// GetSourceByID returns a source by ID and organization.
func (r *Repository) GetSourceByID(ctx context.Context, sourceID, organizationID uint64) (*models.RAGSource, error) {
	var source models.RAGSource
	err := r.db.WithContext(ctx).Where("id = ? AND organization_id = ?", sourceID, organizationID).Take(&source).Error
	if err != nil {
		return nil, err
	}
	return &source, nil
}

// GetSourceByIDOnly returns a source by ID (no org filter).
func (r *Repository) GetSourceByIDOnly(ctx context.Context, sourceID uint64) (*models.RAGSource, error) {
	var source models.RAGSource
	err := r.db.WithContext(ctx).Where("id = ?", sourceID).Take(&source).Error
	if err != nil {
		return nil, err
	}
	return &source, nil
}

// GetSourceInGroup returns a source belonging to a specific group.
func (r *Repository) GetSourceInGroup(ctx context.Context, sourceID, organizationID, groupID uint64) (*models.RAGSource, error) {
	var source models.RAGSource
	err := r.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND source_group_id = ?", sourceID, organizationID, groupID).Take(&source).Error
	if err != nil {
		return nil, err
	}
	return &source, nil
}

// ListSources returns sources filtered by organization, optional conversation, and optional status.
func (r *Repository) ListSources(ctx context.Context, organizationID uint64, conversationID *uint64, status string) ([]models.RAGSource, error) {
	query := r.db.WithContext(ctx).Where("organization_id = ?", organizationID)
	if conversationID != nil {
		query = query.Where("(conversation_id IS NULL OR conversation_id = ?)", *conversationID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var sources []models.RAGSource
	err := query.
		Order("updated_at DESC, id DESC").
		Limit(100).
		Find(&sources).Error
	return sources, err
}

// UpdateSourceFields updates arbitrary fields on a RAGSource.
func (r *Repository) UpdateSourceFields(ctx context.Context, sourceID uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", sourceID).Updates(fields).Error
}

// UpdateSourceByOrgAndGroup updates source fields filtered by org and group.
func (r *Repository) UpdateSourceByOrgAndGroup(ctx context.Context, organizationID, groupID uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.RAGSource{}).
		Where("organization_id = ? AND source_group_id = ?", organizationID, groupID).
		Updates(fields).Error
}

// UpdateSourceDedupeStatus updates the dedupe_status of a source.
func (r *Repository) UpdateSourceDedupeStatus(ctx context.Context, sourceID uint64, dedupeStatus string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", sourceID).Updates(map[string]any{
		"dedupe_status": dedupeStatus,
		"updated_at":    now,
	}).Error
}

// UpdateSourceGroupAndCanonical updates source group and canonical references.
func (r *Repository) UpdateSourceGroupAndCanonical(ctx context.Context, sourceID uint64, groupID uint64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", sourceID).Updates(map[string]any{
		"source_group_id":     groupID,
		"canonical_source_id": sourceID,
		"dedupe_status":       models.RAGSourceDedupeStatusUnique,
		"updated_at":          now,
	}).Error
}

// UpdateSourceActiveVersion sets the active version on a source.
func (r *Repository) UpdateSourceActiveVersion(ctx context.Context, sourceID, versionID uint64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", sourceID).Updates(map[string]any{
		"status":            models.RAGSourceStatusReady,
		"active_version_id": versionID,
		"last_error":        "",
		"updated_at":        now,
	}).Error
}

// MarkSourceFailed sets source status to failed with an error message.
func (r *Repository) MarkSourceFailed(ctx context.Context, sourceID uint64, message string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", sourceID).Updates(map[string]any{
		"status":     models.RAGSourceStatusFailed,
		"last_error": message,
		"updated_at": now,
	}).Error
}

// LoadSourcesByID returns sources for a set of IDs.
func (r *Repository) LoadSourcesByID(ctx context.Context, ids map[uint64]bool) (map[uint64]models.RAGSource, error) {
	out := map[uint64]models.RAGSource{}
	if len(ids) == 0 {
		return out, nil
	}
	var values []uint64
	for id := range ids {
		values = append(values, id)
	}
	var rows []models.RAGSource
	if err := r.db.WithContext(ctx).Where("id IN ?", values).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

// ---------- RAGSourceGroup ----------

// CreateSourceGroup persists a new RAGSourceGroup.
func (r *Repository) CreateSourceGroup(ctx context.Context, group *models.RAGSourceGroup) error {
	return r.db.WithContext(ctx).Create(group).Error
}

// GetSourceGroupByID returns a source group by ID and organization.
func (r *Repository) GetSourceGroupByID(ctx context.Context, groupID, organizationID uint64) (*models.RAGSourceGroup, error) {
	var group models.RAGSourceGroup
	err := r.db.WithContext(ctx).Where("id = ? AND organization_id = ?", groupID, organizationID).Take(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// ListSourceGroups returns source groups for an organization.
func (r *Repository) ListSourceGroups(ctx context.Context, organizationID uint64) ([]models.RAGSourceGroup, error) {
	var groups []models.RAGSourceGroup
	err := r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("updated_at DESC, id DESC").
		Limit(100).
		Find(&groups).Error
	return groups, err
}

// ListSourcesInGroup returns all sources in a source group.
func (r *Repository) ListSourcesInGroup(ctx context.Context, organizationID, groupID uint64) ([]models.RAGSource, error) {
	var sources []models.RAGSource
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND source_group_id = ?", organizationID, groupID).
		Order("id ASC").
		Find(&sources).Error
	return sources, err
}

// UpdateSourceGroupFields updates arbitrary fields on a RAGSourceGroup.
func (r *Repository) UpdateSourceGroupFields(ctx context.Context, groupID, organizationID uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.RAGSourceGroup{}).
		Where("id = ? AND organization_id = ?", groupID, organizationID).
		Updates(fields).Error
}

// ---------- RAGSourceVersion ----------

// GetVersionByID returns a version by ID.
func (r *Repository) GetVersionByID(ctx context.Context, versionID uint64) (*models.RAGSourceVersion, error) {
	var version models.RAGSourceVersion
	err := r.db.WithContext(ctx).Where("id = ?", versionID).Take(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// GetPendingVersionBySource returns the latest pending version for a source.
func (r *Repository) GetPendingVersionBySource(ctx context.Context, sourceID uint64) (*models.RAGSourceVersion, error) {
	var version models.RAGSourceVersion
	err := r.db.WithContext(ctx).
		Where("source_id = ? AND status = ?", sourceID, models.RAGSourceVersionStatusPending).
		Order("version DESC").
		Take(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// GetActiveVersionBySource returns the latest active version for a source.
func (r *Repository) GetActiveVersionBySource(ctx context.Context, sourceID uint64) (*models.RAGSourceVersion, error) {
	var version models.RAGSourceVersion
	err := r.db.WithContext(ctx).
		Where("source_id = ? AND status = ?", sourceID, models.RAGSourceVersionStatusActive).
		Order("version DESC").
		Take(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// CreateVersion persists a new RAGSourceVersion.
func (r *Repository) CreateVersion(ctx context.Context, version *models.RAGSourceVersion) error {
	return r.db.WithContext(ctx).Create(version).Error
}

// NextVersionNumber returns the next version number for a source.
func (r *Repository) NextVersionNumber(ctx context.Context, sourceID uint64) (int, error) {
	var latest models.RAGSourceVersion
	err := r.db.WithContext(ctx).Where("source_id = ?", sourceID).Order("version DESC").Take(&latest).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 1, nil
		}
		return 0, err
	}
	return latest.Version + 1, nil
}

// NextVersionNumberTx returns the next version number within a transaction.
func (r *Repository) NextVersionNumberTx(ctx context.Context, tx *gorm.DB, sourceID uint64) (int, error) {
	var latest models.RAGSourceVersion
	err := tx.WithContext(ctx).Where("source_id = ?", sourceID).Order("version DESC").Take(&latest).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 1, nil
		}
		return 0, err
	}
	return latest.Version + 1, nil
}

// ListVersionsBySource returns versions for a source.
func (r *Repository) ListVersionsBySource(ctx context.Context, sourceID uint64) ([]models.RAGSourceVersion, error) {
	var versions []models.RAGSourceVersion
	err := r.db.WithContext(ctx).Where("source_id = ?", sourceID).Order("version DESC").Find(&versions).Error
	return versions, err
}

// LoadVersionsByID returns versions for a set of IDs.
func (r *Repository) LoadVersionsByID(ctx context.Context, ids map[uint64]bool) (map[uint64]models.RAGSourceVersion, error) {
	out := map[uint64]models.RAGSourceVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	var values []uint64
	for id := range ids {
		values = append(values, id)
	}
	var rows []models.RAGSourceVersion
	if err := r.db.WithContext(ctx).Where("id IN ?", values).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

// MarkVersionSuperseded sets version status to superseded.
func (r *Repository) MarkVersionSuperseded(ctx context.Context, versionID uint64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSourceVersion{}).Where("id = ?", versionID).Updates(map[string]any{
		"status":     models.RAGSourceVersionStatusSuperseded,
		"updated_at": now,
	}).Error
}

// MarkAllActiveVersionsSuperseded supersedes all active versions for a source.
func (r *Repository) MarkAllActiveVersionsSuperseded(ctx context.Context, sourceID uint64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSourceVersion{}).
		Where("source_id = ? AND status = ?", sourceID, models.RAGSourceVersionStatusActive).
		Updates(map[string]any{"status": models.RAGSourceVersionStatusSuperseded, "updated_at": now}).Error
}

// ActivateVersion marks a version as active with content metadata.
func (r *Repository) ActivateVersion(ctx context.Context, versionID uint64, contentHash string, simHash uint64, rawText string, chunkCount int, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSourceVersion{}).Where("id = ?", versionID).Updates(map[string]any{
		"content_hash":    contentHash,
		"normalized_hash": contentHash,
		"sim_hash64":      simHash,
		"raw_text":        rawText,
		"status":          models.RAGSourceVersionStatusActive,
		"chunk_count":     chunkCount,
		"last_error":      "",
		"activated_at":    now,
		"updated_at":      now,
	}).Error
}

// MarkVersionFailed sets version status to failed.
func (r *Repository) MarkVersionFailed(ctx context.Context, versionID uint64, message string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSourceVersion{}).Where("id = ?", versionID).Updates(map[string]any{
		"status":     models.RAGSourceVersionStatusFailed,
		"last_error": message,
		"updated_at": now,
	}).Error
}

// ---------- RAGChunk ----------

// GetChunkByID returns a chunk by ID.
func (r *Repository) GetChunkByID(ctx context.Context, chunkID uint64) (*models.RAGChunk, error) {
	var chunk models.RAGChunk
	err := r.db.WithContext(ctx).Where("id = ?", chunkID).Take(&chunk).Error
	if err != nil {
		return nil, err
	}
	return &chunk, nil
}

// ListChunksBySource returns chunks for a source.
func (r *Repository) ListChunksBySource(ctx context.Context, sourceID uint64) ([]models.RAGChunk, error) {
	var chunks []models.RAGChunk
	err := r.db.WithContext(ctx).Where("source_id = ?", sourceID).Order("source_version_id DESC, chunk_index ASC").Limit(200).Find(&chunks).Error
	return chunks, err
}

// ListChunksByVersion returns chunks for a version.
func (r *Repository) ListChunksByVersion(ctx context.Context, versionID uint64) ([]models.RAGChunk, error) {
	var chunks []models.RAGChunk
	err := r.db.WithContext(ctx).Where("source_version_id = ?", versionID).Order("chunk_index ASC").Find(&chunks).Error
	return chunks, err
}

// CreateChunk persists a new RAGChunk.
func (r *Repository) CreateChunk(ctx context.Context, chunk *models.RAGChunk) error {
	return r.db.WithContext(ctx).Create(chunk).Error
}

// DeleteChunksByVersion deletes all chunks for a version.
func (r *Repository) DeleteChunksByVersion(ctx context.Context, versionID uint64) error {
	return r.db.WithContext(ctx).Where("source_version_id = ?", versionID).Delete(&models.RAGChunk{}).Error
}

// UpdateChunkIndexStatus updates chunk indexing status and metadata.
func (r *Repository) UpdateChunkIndexStatus(ctx context.Context, chunkID uint64, indexStatus, lastError string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunkID).Updates(map[string]any{
		"index_status": indexStatus,
		"last_error":   lastError,
		"updated_at":   now,
	}).Error
}

// UpdateChunkIndexedAt marks a chunk as indexed.
func (r *Repository) UpdateChunkIndexedAt(ctx context.Context, chunkID uint64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunkID).Updates(map[string]any{
		"index_status": models.RAGChunkIndexStatusIndexed,
		"last_error":   "",
		"indexed_at":   now,
		"updated_at":   now,
	}).Error
}

// LoadActiveChunks returns chunks joined with sources and versions for search.
func (r *Repository) LoadActiveChunks(ctx context.Context, organizationID uint64, conversationID *uint64) ([]models.RAGChunk, error) {
	query := r.db.WithContext(ctx).
		Joins("JOIN rag_sources ON rag_sources.id = rag_chunks.source_id").
		Joins("JOIN rag_source_versions ON rag_source_versions.id = rag_chunks.source_version_id").
		Joins("LEFT JOIN rag_source_groups ON rag_source_groups.id = rag_sources.source_group_id").
		Where("rag_chunks.organization_id = ? AND rag_sources.status = ? AND rag_source_versions.status = ?", organizationID, models.RAGSourceStatusReady, models.RAGSourceVersionStatusActive).
		Where("(rag_source_groups.id IS NULL OR rag_source_groups.status = ?)", models.RAGSourceGroupStatusActive).
		Where("(rag_sources.dedupe_status IS NULL OR rag_sources.dedupe_status <> ?)", models.RAGSourceDedupeStatusConfirmedDuplicate)
	if conversationID != nil && *conversationID != 0 {
		query = query.Where("(rag_chunks.conversation_id IS NULL OR rag_chunks.conversation_id = ?)", *conversationID)
	} else {
		query = query.Where("rag_chunks.conversation_id IS NULL")
	}
	var chunks []models.RAGChunk
	err := query.Order("rag_chunks.updated_at DESC").Limit(300).Find(&chunks).Error
	return chunks, err
}

// ---------- RAGSourceDuplicate ----------

// ListDuplicateCandidates returns duplicate candidates for an organization.
func (r *Repository) ListDuplicateCandidates(ctx context.Context, organizationID uint64) ([]models.RAGSourceDuplicate, error) {
	var rows []models.RAGSourceDuplicate
	err := r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("status ASC, similarity DESC, updated_at DESC").
		Limit(100).
		Find(&rows).Error
	return rows, err
}

// GetDuplicateCandidate returns a duplicate candidate by ID and organization.
func (r *Repository) GetDuplicateCandidate(ctx context.Context, duplicateID, organizationID uint64) (*models.RAGSourceDuplicate, error) {
	var duplicate models.RAGSourceDuplicate
	err := r.db.WithContext(ctx).Where("id = ? AND organization_id = ?", duplicateID, organizationID).Take(&duplicate).Error
	if err != nil {
		return nil, err
	}
	return &duplicate, nil
}

// UpdateDuplicateCandidateStatus updates the status and decision fields on a duplicate.
func (r *Repository) UpdateDuplicateCandidateStatus(ctx context.Context, duplicateID uint64, status, decision string, decidedBy uint64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.RAGSourceDuplicate{}).Where("id = ?", duplicateID).Updates(map[string]any{
		"status":     status,
		"decision":   decision,
		"decided_by": decidedBy,
		"decided_at": now,
		"updated_at": now,
	}).Error
}

// FindDuplicateCandidateVersions returns candidate version info for duplicate detection.
func (r *Repository) FindDuplicateCandidateVersions(ctx context.Context, organizationID, excludeVersionID uint64) ([]duplicateCandidateRow, error) {
	var candidates []duplicateCandidateRow
	err := r.db.WithContext(ctx).
		Table("rag_source_versions").
		Select("rag_source_versions.source_id, rag_sources.source_group_id, rag_source_versions.content_hash, rag_source_versions.normalized_hash, rag_source_versions.sim_hash64").
		Joins("JOIN rag_sources ON rag_sources.id = rag_source_versions.source_id").
		Where("rag_source_versions.organization_id = ? AND rag_source_versions.status = ? AND rag_source_versions.id <> ?", organizationID, models.RAGSourceVersionStatusActive, excludeVersionID).
		Where("(rag_sources.dedupe_status IS NULL OR rag_sources.dedupe_status <> ?)", models.RAGSourceDedupeStatusConfirmedDuplicate).
		Find(&candidates).Error
	return candidates, err
}

// UpsertDuplicateCandidate creates or finds an existing duplicate candidate.
func (r *Repository) UpsertDuplicateCandidate(ctx context.Context, duplicate *models.RAGSourceDuplicate) error {
	return r.db.WithContext(ctx).Where(
		"organization_id = ? AND source_id = ? AND candidate_source_id = ?",
		duplicate.OrganizationID,
		duplicate.SourceID,
		duplicate.CandidateSourceID,
	).FirstOrCreate(duplicate).Error
}

// ---------- EventOutbox ----------

// ListRAGDeadLetters returns failed RAG-related outbox events.
func (r *Repository) ListRAGDeadLetters(ctx context.Context) ([]models.EventOutbox, error) {
	var rows []models.EventOutbox
	err := r.db.WithContext(ctx).
		Where("status = ? AND event IN ?", models.EventOutboxStatusFailed, []string{EventSourceIngestRequested, EventChunkIndexRequested}).
		Order("updated_at DESC").
		Limit(100).
		Find(&rows).Error
	return rows, err
}

// GetDeadLetterByID returns a failed outbox event by ID.
func (r *Repository) GetDeadLetterByID(ctx context.Context, eventID uint64) (*models.EventOutbox, error) {
	var row models.EventOutbox
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ? AND event IN ?", eventID, models.EventOutboxStatusFailed, []string{EventSourceIngestRequested, EventChunkIndexRequested}).
		Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ResetDeadLetterToPending requeues a dead letter for retry.
func (r *Repository) ResetDeadLetterToPending(ctx context.Context, eventID uint64, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.EventOutbox{}).Where("id = ?", eventID).Updates(map[string]any{
			"status":     models.EventOutboxStatusPending,
			"attempts":   0,
			"last_error": "",
			"locked_by":  "",
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.EventOutbox{}).Where("id = ?", eventID).UpdateColumn("available_at", gorm.Expr("NULL")).Error; err != nil {
			return err
		}
		return tx.Model(&models.EventOutbox{}).Where("id = ?", eventID).UpdateColumn("locked_until", gorm.Expr("NULL")).Error
	})
}

// ---------- Transaction Helpers ----------

// Run executes a function within a database transaction.
func (r *Repository) Run(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// DB returns the underlying gorm.DB for direct transaction access.
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// ---------- Types ----------

// duplicateCandidateRow is the shape returned by FindDuplicateCandidateVersions.
type duplicateCandidateRow struct {
	SourceID       uint64
	SourceGroupID  *uint64
	ContentHash    string
	NormalizedHash string
	SimHash64      uint64
}
