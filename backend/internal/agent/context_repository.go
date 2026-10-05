package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

// contextRepository loads bounded conversation context in at most five SQL
// statements before optional chunk retrieval:
//
//  1. conversations LEFT JOIN contact_profiles (single row, profile presence
//     distinguishes skipped/not_found/found)
//  2. messages ordered newest-first with budget.Messages LIMIT
//  3. conversation_members ordered by id
//  4. one UNION ALL query for notes, memories, rooms, and followups
//  5. one UNION ALL query for call transcript segments, meeting transcript
//     segments, and latest recording-transcription status
//
// The optional retrieval phase (refreshConversationContextChunks /
// retrieveConversationContextChunks) is outside LoadBase and not counted.
type contextRepository struct {
	db *gorm.DB
}

// LoadBase loads the core conversation context using at most five SQL
// statements and returns the context, the actual statement count, and any
// error. Every real database operation increments the internal counter so
// the returned count reflects actual SQL statements executed.
func (r contextRepository) LoadBase(ctx context.Context, organizationID, userID, conversationID uint64, budget ContextBudget) (*conversationContext, int, error) {
	var queryCount int

	// Statement 1: conversation LEFT JOIN contact_profiles.
	// A single query loads the conversation row and, when the conversation
	// has a contact_id, the matching contact profile. If no row matches the
	// JOIN, the profile columns are NULL, which distinguishes "not_found"
	// from "skipped" (no contact_id on the conversation).
	conv, contactProfile, contactProfileLookupAttempted, err := r.loadConversationWithProfile(ctx, organizationID, userID, conversationID)
	if err != nil {
		return nil, queryCount, err
	}
	queryCount++

	// Statement 2: messages ordered newest-first with budget limit.
	var messages []models.Message
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(budget.Messages).
		Find(&messages).Error; err != nil {
		return nil, queryCount, err
	}
	queryCount++
	decryptMessageBodies(messages)

	// Statement 3: conversation members ordered by id.
	var members []models.ConversationMember
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&members).Error; err != nil {
		return nil, queryCount, err
	}
	queryCount++

	// Statement 4: one UNION ALL query for notes, memories, rooms, and
	// followups. Followups are filtered by call IDs extracted from the
	// already-bounded message list.
	callIDs := extractCallIDsFromMessages(messages)
	notes, memories, rooms, followups, err := r.loadTaggedArtifactsUnion(ctx, organizationID, conversationID, callIDs, budget)
	if err != nil {
		return nil, queryCount, err
	}
	queryCount++

	// Statement 5: one UNION ALL query for call transcript segments,
	// meeting transcript segments, and latest recording-transcription status.
	callTranscriptSegments, meetingTranscriptSegments, latestRecordingTranscription, err := r.loadTranscriptArtifactsUnion(ctx, organizationID, conversationID, callIDs, budget)
	if err != nil {
		return nil, queryCount, err
	}
	queryCount++

	var contactProfilePtr *models.ContactProfile
	if contactProfile != nil {
		contactProfilePtr = contactProfile
	}

	conversationCtx := &conversationContext{
		Conversation:                  *conv,
		Notes:                         notes,
		Messages:                      messages,
		Rooms:                         rooms,
		Members:                       members,
		Memories:                      memories,
		Followups:                     followups,
		TranscriptSegments:            callTranscriptSegments,
		MeetingTranscriptSegments:     meetingTranscriptSegments,
		ContactProfile:                contactProfilePtr,
		ContactProfileLookupAttempted: contactProfileLookupAttempted,
		Manifest: ContextManifest{
			Selected: map[string]int{
				"messages":                    len(messages),
				"notes":                       len(notes),
				"memories":                    len(memories),
				"rooms":                       len(rooms),
				"followups":                   len(followups),
				"call_transcript_segments":     len(callTranscriptSegments),
				"meeting_transcript_segments": len(meetingTranscriptSegments),
				"members":                     len(members),
			},
			SQLStatements: queryCount,
		},
	}
	conversationCtx.MeetingContext = buildMeetingContextSummary(
		conversationCtx.TranscriptSegments,
		conversationCtx.Followups,
		conversationCtx.MeetingTranscriptSegments,
		latestRecordingTranscription,
	)
	prioritizeMeetingConversationArtifacts(conversationCtx)
	return conversationCtx, queryCount, nil
}

// conversationProfileRow is the scan target for the LEFT JOIN query that
// loads a conversation together with its optional contact profile in a
// single SQL statement.
type conversationProfileRow struct {
	models.Conversation
	// Contact profile columns — NULL when the conversation has no contact_id
	// or when no matching profile exists.
	ProfileID                sql.NullInt64
	ProfileOrganizationID    sql.NullInt64
	ProfileOwnerID           sql.NullInt64
	ProfileContactUserID     sql.NullInt64
	ProfileCompany           sql.NullString
	ProfileRole              sql.NullString
	ProfileTimezone          sql.NullString
	ProfileDefaultSourceLang sql.NullString
	ProfileDefaultTargetLang sql.NullString
	ProfileRelationship      sql.NullString
	ProfileContactStart      sql.NullString
	ProfileContactEnd        sql.NullString
	ProfileContactDays       sql.NullString
	ProfileLastFollowupState sql.NullString
	ProfileNote              sql.NullString
	ProfileCreatedAt         sql.NullTime
	ProfileUpdatedAt         sql.NullTime
}

// loadConversationWithProfile loads the conversation and its optional contact
// profile in a single LEFT JOIN query. It returns the conversation, the
// profile (nil if not found or skipped), whether the lookup was attempted,
// and any error.
func (r contextRepository) loadConversationWithProfile(ctx context.Context, organizationID, userID, conversationID uint64) (*models.Conversation, *models.ContactProfile, bool, error) {
	var row conversationProfileRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.*,
			cp.id AS profile_id,
			cp.organization_id AS profile_organization_id,
			cp.owner_id AS profile_owner_id,
			cp.contact_user_id AS profile_contact_user_id,
			cp.company AS profile_company,
			cp.role AS profile_role,
			cp.timezone AS profile_timezone,
			cp.default_source_lang AS profile_default_source_lang,
			cp.default_target_lang AS profile_default_target_lang,
			cp.relationship_status AS profile_relationship,
			cp.preferred_contact_start AS profile_contact_start,
			cp.preferred_contact_end AS profile_contact_end,
			cp.preferred_contact_days AS profile_contact_days,
			cp.last_followup_state AS profile_last_followup_state,
			cp.note AS profile_note,
			cp.created_at AS profile_created_at,
			cp.updated_at AS profile_updated_at
		FROM conversations c
		LEFT JOIN contact_profiles cp ON cp.organization_id = c.organization_id
			AND cp.owner_id = ?
			AND cp.contact_user_id = c.contact_id
		WHERE c.organization_id = ? AND c.id = ?`,
		userID, organizationID, conversationID).Scan(&row).Error
	if err != nil {
		return nil, nil, false, err
	}
	if row.ID == 0 {
		return nil, nil, false, gorm.ErrRecordNotFound
	}

	conv := row.Conversation
	contactProfileLookupAttempted := conv.ContactID != nil && *conv.ContactID != 0

	var profile *models.ContactProfile
	if row.ProfileID.Valid && row.ProfileID.Int64 != 0 {
		profile = &models.ContactProfile{
			ID:                    uint64(row.ProfileID.Int64),
			OrganizationID:        uint64(row.ProfileOrganizationID.Int64),
			OwnerID:               uint64(row.ProfileOwnerID.Int64),
			ContactUserID:         uint64(row.ProfileContactUserID.Int64),
			Company:               row.ProfileCompany.String,
			Role:                  row.ProfileRole.String,
			Timezone:              row.ProfileTimezone.String,
			DefaultSourceLang:     row.ProfileDefaultSourceLang.String,
			DefaultTargetLang:     row.ProfileDefaultTargetLang.String,
			RelationshipStatus:    row.ProfileRelationship.String,
			PreferredContactStart: row.ProfileContactStart.String,
			PreferredContactEnd:   row.ProfileContactEnd.String,
			PreferredContactDays:  row.ProfileContactDays.String,
			LastFollowupState:     row.ProfileLastFollowupState.String,
			Note:                  row.ProfileNote.String,
		}
		if row.ProfileCreatedAt.Valid {
			profile.CreatedAt = row.ProfileCreatedAt.Time
		}
		if row.ProfileUpdatedAt.Valid {
			profile.UpdatedAt = row.ProfileUpdatedAt.Time
		}
	}
	return &conv, profile, contactProfileLookupAttempted, nil
}

// taggedArtifactRow is the scan target for the UNION ALL query that loads
// notes, memories, rooms, and followups in a single SQL statement.
type taggedArtifactRow struct {
	Tag int
	// Common fields
	ID             uint64
	OrganizationID uint64
	ConversationID uint64
	SortAt         time.Time
	// Note fields
	AuthorID uint64
	Body     string
	// Memory fields
	UserID      uint64
	Scope       string
	Key         string
	MemoryType  string
	Importance  int
	SourceType  string
	SourceRefID uint64
	ValueJSON   string
	LastRunID   uint64
	// Room fields
	Title     string
	Status    string
	CreatedBy uint64
	// Followup fields
	CallID     string
	PeerUserID uint64
	SummaryEN  string
	GeneratedAt sql.NullTime
}

// loadTaggedArtifactsUnion loads notes, memories, rooms, and followups in a
// single UNION ALL query. Each subquery is wrapped in parentheses with its
// own ORDER BY and LIMIT, and the outer query preserves the tag for decoding.
func (r contextRepository) loadTaggedArtifactsUnion(ctx context.Context, organizationID, conversationID uint64, callIDs []string, budget ContextBudget) ([]models.ConversationNote, []models.AgentMemory, []models.CallRoom, []models.CallFollowup, error) {
	// Build the followup subquery conditionally.
	followupSubquery := fmt.Sprintf(
		`SELECT * FROM (SELECT %d AS tag, id, organization_id, 0 AS conversation_id, COALESCE(generated_at, updated_at) AS sort_at, 0 AS author_id, '' AS body, 0 AS user_id, '' AS scope, '' AS key, '' AS memory_type, 0 AS importance, '' AS source_type, 0 AS source_ref_id, '' AS value_json, 0 AS last_run_id, '' AS title, '' AS status, 0 AS created_by, call_id, peer_user_id, summary_en, generated_at FROM call_followups WHERE call_id IN (?) AND (organization_id = ? OR organization_id = 0) ORDER BY generated_at DESC, updated_at DESC LIMIT %d)`,
		4, budget.Followups,
	)
	followupArgs := []any{callIDs, organizationID}
	if len(callIDs) == 0 {
		// When no call IDs, use a subquery that returns zero rows.
		followupSubquery = fmt.Sprintf(
			`SELECT * FROM (SELECT %d AS tag, id, organization_id, 0 AS conversation_id, updated_at AS sort_at, 0 AS author_id, '' AS body, 0 AS user_id, '' AS scope, '' AS key, '' AS memory_type, 0 AS importance, '' AS source_type, 0 AS source_ref_id, '' AS value_json, 0 AS last_run_id, '' AS title, '' AS status, 0 AS created_by, '' AS call_id, 0 AS peer_user_id, '' AS summary_en, NULL AS generated_at FROM call_followups WHERE 1=0 LIMIT 0)`,
			4,
		)
		followupArgs = nil
	}

	query := fmt.Sprintf(`
		SELECT * FROM (SELECT %d AS tag, id, organization_id, conversation_id, created_at AS sort_at, author_id, body, 0 AS user_id, '' AS scope, '' AS key, '' AS memory_type, 0 AS importance, '' AS source_type, 0 AS source_ref_id, '' AS value_json, 0 AS last_run_id, '' AS title, '' AS status, 0 AS created_by, '' AS call_id, 0 AS peer_user_id, '' AS summary_en, NULL AS generated_at FROM conversation_notes WHERE organization_id = ? AND conversation_id = ? ORDER BY created_at DESC LIMIT %d)
		UNION ALL
		SELECT * FROM (SELECT %d AS tag, id, organization_id, conversation_id, updated_at AS sort_at, 0 AS author_id, '' AS body, user_id, scope, key, memory_type, importance, source_type, source_ref_id, value_json, last_run_id, '' AS title, '' AS status, 0 AS created_by, '' AS call_id, 0 AS peer_user_id, '' AS summary_en, NULL AS generated_at FROM agent_memories WHERE organization_id = ? AND conversation_id = ? ORDER BY updated_at DESC LIMIT %d)
		UNION ALL
		SELECT * FROM (SELECT %d AS tag, id, organization_id, 0 AS conversation_id, created_at AS sort_at, 0 AS author_id, '' AS body, 0 AS user_id, '' AS scope, '' AS key, '' AS memory_type, 0 AS importance, '' AS source_type, 0 AS source_ref_id, '' AS value_json, 0 AS last_run_id, title, status, created_by, '' AS call_id, 0 AS peer_user_id, '' AS summary_en, NULL AS generated_at FROM call_rooms WHERE organization_id = ? AND conversation_id = ? ORDER BY created_at DESC LIMIT %d)
		UNION ALL
		%s`,
		1, budget.Notes,
		2, budget.Memories,
		3, budget.Rooms,
		followupSubquery,
	)

	var args []any
	args = append(args, organizationID, conversationID)   // notes
	args = append(args, organizationID, conversationID)   // memories
	args = append(args, organizationID, conversationID)   // rooms
	args = append(args, followupArgs...)                   // followups

	var rows []taggedArtifactRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, nil, nil, nil, err
	}

	var notes []models.ConversationNote
	var memories []models.AgentMemory
	var rooms []models.CallRoom
	var followups []models.CallFollowup

	for _, row := range rows {
		switch row.Tag {
		case 1: // note
			notes = append(notes, models.ConversationNote{
				ID:             row.ID,
				OrganizationID: row.OrganizationID,
				ConversationID: row.ConversationID,
				AuthorID:       row.AuthorID,
				Body:           row.Body,
				CreatedAt:      row.SortAt,
			})
		case 2: // memory
			memories = append(memories, models.AgentMemory{
				ID:             row.ID,
				OrganizationID: row.OrganizationID,
				UserID:         row.UserID,
				ConversationID: row.ConversationID,
				Scope:          row.Scope,
				Key:            row.Key,
				MemoryType:     row.MemoryType,
				Importance:     row.Importance,
				SourceType:     row.SourceType,
				SourceRefID:    row.SourceRefID,
				ValueJSON:      row.ValueJSON,
				LastRunID:      row.LastRunID,
				UpdatedAt:      row.SortAt,
			})
		case 3: // room
			rooms = append(rooms, models.CallRoom{
				ID:             row.ID,
				OrganizationID: row.OrganizationID,
				ConversationID: &row.ConversationID,
				Title:          row.Title,
				Status:         row.Status,
				CreatedBy:      row.CreatedBy,
				CreatedAt:      row.SortAt,
			})
		case 4: // followup
			followups = append(followups, models.CallFollowup{
				ID:             row.ID,
				OrganizationID: row.OrganizationID,
				CallID:         row.CallID,
				PeerUserID:     row.PeerUserID,
				SummaryEN:      row.SummaryEN,
				GeneratedAt:    func() *time.Time { if row.GeneratedAt.Valid { t := row.GeneratedAt.Time; return &t }; return nil }(),
			})
		}
	}
	return notes, memories, rooms, followups, nil
}

// transcriptArtifactRow is the scan target for the UNION ALL query that loads
// call transcript segments, meeting transcript segments, and the latest
// recording-transcription status.
type transcriptArtifactRow struct {
	Tag     int
	ID      uint64
	Content string
	SortKey string
	// Call transcript fields
	CallID     string
	UserID     uint64
	PeerUserID uint64
	FromEmail  string
	ToEmail    string
	SourceLang string
	TargetLang string
	TimestampMS int64
	// Meeting transcript fields
	OrganizationID     uint64
	ConversationID     uint64
	RoomID             uint64
	RecordingSessionID uint64
	RecordingFileID    uint64
	SpeakerUserID      sql.NullInt64
	TrackKey           string
	Provider           string
	Language           string
	StartMS            int64
	EndMS              int64
	// Recording transcription fields
	Status       string
	SegmentCount int
}

// loadTranscriptArtifactsUnion loads call transcript segments, meeting
// transcript segments, and the latest recording-transcription status in a
// single UNION ALL query. The recording-transcription row (tag 3) provides
// the status for buildMeetingContextSummary.
func (r contextRepository) loadTranscriptArtifactsUnion(ctx context.Context, organizationID, conversationID uint64, callIDs []string, budget ContextBudget) ([]models.CallTranscriptSegment, []models.MeetingTranscriptSegment, models.RecordingTranscription, error) {
	callSubquery := `SELECT * FROM (SELECT 0 AS tag, id, '' AS content, '' AS sort_key, call_id, user_id, peer_user_id, from_email, to_email, original_text, source_lang, target_lang, timestamp_ms, 0 AS organization_id, 0 AS conversation_id, 0 AS room_id, 0 AS recording_session_id, 0 AS recording_file_id, NULL AS speaker_user_id, '' AS track_key, '' AS provider, '' AS language, 0 AS start_ms, 0 AS end_ms, '' AS status, 0 AS segment_count FROM call_transcript_segments WHERE 1=0 LIMIT 0)`
	callArgs := []any{}
	if len(callIDs) > 0 {
		callSubquery = fmt.Sprintf(
			`SELECT * FROM (SELECT 1 AS tag, id, original_text AS content, '' AS sort_key, call_id, user_id, peer_user_id, from_email, to_email, original_text, source_lang, target_lang, timestamp_ms, 0 AS organization_id, 0 AS conversation_id, 0 AS room_id, 0 AS recording_session_id, 0 AS recording_file_id, NULL AS speaker_user_id, '' AS track_key, '' AS provider, '' AS language, 0 AS start_ms, 0 AS end_ms, '' AS status, 0 AS segment_count FROM call_transcript_segments WHERE call_id IN (?) ORDER BY timestamp_ms DESC, created_at DESC LIMIT %d)`,
			budget.CallTranscriptSegments,
		)
		callArgs = []any{callIDs}
	}

	query := fmt.Sprintf(`
		%s
		UNION ALL
		SELECT * FROM (SELECT 2 AS tag, id, text AS content, '' AS sort_key, '' AS call_id, 0 AS user_id, 0 AS peer_user_id, '' AS from_email, '' AS to_email, '' AS original_text, '' AS source_lang, '' AS target_lang, 0 AS timestamp_ms, organization_id, conversation_id, room_id, recording_session_id, recording_file_id, speaker_user_id, track_key, provider, language, start_ms, end_ms, '' AS status, 0 AS segment_count FROM meeting_transcript_segments WHERE organization_id = ? AND conversation_id = ? ORDER BY recording_session_id DESC, start_ms ASC, created_at DESC LIMIT %d)
		UNION ALL
		SELECT * FROM (SELECT 3 AS tag, id, '' AS content, '' AS sort_key, '' AS call_id, 0 AS user_id, 0 AS peer_user_id, '' AS from_email, '' AS to_email, '' AS original_text, '' AS source_lang, '' AS target_lang, 0 AS timestamp_ms, organization_id, 0 AS conversation_id, 0 AS room_id, recording_session_id, 0 AS recording_file_id, NULL AS speaker_user_id, '' AS track_key, '' AS provider, '' AS language, 0 AS start_ms, 0 AS end_ms, status, segment_count FROM recording_transcriptions WHERE organization_id = ? AND conversation_id = ? ORDER BY recording_session_id DESC, updated_at DESC LIMIT 1)`,
		callSubquery,
		budget.MeetingTranscriptSegments,
	)

	var args []any
	args = append(args, callArgs...)                     // call transcript segments
	args = append(args, organizationID, conversationID)  // meeting transcript segments
	args = append(args, organizationID, conversationID)  // recording transcription

	var rows []transcriptArtifactRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, nil, models.RecordingTranscription{}, err
	}

	var callSegments []models.CallTranscriptSegment
	var meetingSegments []models.MeetingTranscriptSegment
	var transcription models.RecordingTranscription

	for _, row := range rows {
		switch row.Tag {
		case 1: // call transcript segment
			callSegments = append(callSegments, models.CallTranscriptSegment{
				ID:           row.ID,
				CallID:       row.CallID,
				UserID:       row.UserID,
				PeerUserID:   row.PeerUserID,
				FromEmail:    row.FromEmail,
				ToEmail:      row.ToEmail,
				OriginalText: row.Content,
				SourceLang:   row.SourceLang,
				TargetLang:   row.TargetLang,
				TimestampMS:  row.TimestampMS,
			})
		case 2: // meeting transcript segment
			speakerUserID := uint64(0)
			if row.SpeakerUserID.Valid {
				speakerUserID = uint64(row.SpeakerUserID.Int64)
			}
			meetingSegments = append(meetingSegments, models.MeetingTranscriptSegment{
				ID:                 row.ID,
				OrganizationID:     row.OrganizationID,
				ConversationID:     row.ConversationID,
				RoomID:             row.RoomID,
				RecordingSessionID: row.RecordingSessionID,
				RecordingFileID:    row.RecordingFileID,
				SpeakerUserID:      &speakerUserID,
				TrackKey:           row.TrackKey,
				Source:             models.MeetingTranscriptSourceRecording,
				Provider:           row.Provider,
				Language:           row.Language,
				Text:               row.Content,
				StartMS:            row.StartMS,
				EndMS:              row.EndMS,
			})
		case 3: // recording transcription
			transcription = models.RecordingTranscription{
				ID:                 row.ID,
				OrganizationID:     row.OrganizationID,
				RecordingSessionID: row.RecordingSessionID,
				Status:             row.Status,
				SegmentCount:       row.SegmentCount,
			}
		}
	}
	return callSegments, meetingSegments, transcription, nil
}

// observeContextAssembly records context assembly metrics through Task 1's
// ObserveAgentContext function. It uses the manifest to report actual
// collection sizes and the serialized byte/token counts.
func observeContextAssembly(start time.Time, manifest ContextManifest, budget ContextBudget) {
	duration := time.Since(start)
	entries := 0
	for _, count := range manifest.Selected {
		entries += count
	}
	metrics.ObserveAgentContext(duration, entries, manifest.Selected["chunks"], budget.MaxEstimatedTokens, manifest.EstimatedTokens)
}

// estimateContextTokens uses the same heuristic as estimatePromptTokens:
// roughly 4 characters per token, with a word-count floor.
func estimateContextTokens(conversationCtx *conversationContext) int {
	var b strings.Builder
	for _, msg := range conversationCtx.Messages {
		b.WriteString(msg.Body)
		b.WriteByte(' ')
	}
	for _, note := range conversationCtx.Notes {
		b.WriteString(note.Body)
		b.WriteByte(' ')
	}
	for _, mem := range conversationCtx.Memories {
		b.WriteString(mem.ValueJSON)
		b.WriteByte(' ')
	}
	for _, seg := range conversationCtx.TranscriptSegments {
		b.WriteString(seg.OriginalText)
		b.WriteByte(' ')
	}
	for _, seg := range conversationCtx.MeetingTranscriptSegments {
		b.WriteString(seg.Text)
		b.WriteByte(' ')
	}
	for _, chunk := range conversationCtx.ContextChunks {
		b.WriteString(retrievedChunkContent(chunk))
		b.WriteByte(' ')
	}
	if conversationCtx.ContactProfile != nil {
		b.WriteString(buildContactProfileContextContent(*conversationCtx.ContactProfile))
	}
	joined := b.String()
	runeEstimate := (len([]rune(joined)) + 3) / 4
	wordEstimate := len(strings.Fields(joined))
	if runeEstimate > wordEstimate {
		return runeEstimate
	}
	if wordEstimate > 0 {
		return wordEstimate
	}
	return 1
}

// applyContextBudget trims collections that exceed the byte or token budget.
// It preserves the goal and at least one current-message source when present.
// Collections are trimmed in this order:
//  1. old meeting transcript segments
//  2. old call transcript segments
//  3. old messages (but never the last one)
//  4. low-importance memories
//  5. old notes
//  6. low-score context chunks
func applyContextBudget(conversationCtx *conversationContext, budget ContextBudget) {
	if conversationCtx == nil {
		return
	}
	estimatedTokens := estimateContextTokens(conversationCtx)
	conversationCtx.Manifest.EstimatedTokens = estimatedTokens

	// Serialize the runtime request once to measure actual bytes.
	request := buildAgentRuntimeRequest(models.AgentRun{
		OrganizationID: conversationCtx.Conversation.OrganizationID,
		UserID:         0,
		ConversationID: conversationCtx.Conversation.ID,
	}, "budget_check", conversationCtx)
	raw, err := json.Marshal(request)
	if err == nil {
		conversationCtx.Manifest.SerializedBytes = len(raw)
	}

	// Check if we're within budget.
	if conversationCtx.Manifest.SerializedBytes <= budget.MaxBytes && estimatedTokens <= budget.MaxEstimatedTokens {
		return
	}

	// Trim in priority order. After each trim step, re-estimate tokens from
	// the reduced collection sizes (cheap) and only re-serialize at the end
	// to confirm the final byte count.
	trimSteps := []struct {
		name string
		trim func()
	}{
		{"meeting_transcript_segments", func() {
			if len(conversationCtx.MeetingTranscriptSegments) > 1 {
				conversationCtx.MeetingTranscriptSegments = conversationCtx.MeetingTranscriptSegments[:1]
				conversationCtx.Manifest.Selected["meeting_transcript_segments"] = len(conversationCtx.MeetingTranscriptSegments)
			}
		}},
		{"call_transcript_segments", func() {
			if len(conversationCtx.TranscriptSegments) > 1 {
				conversationCtx.TranscriptSegments = conversationCtx.TranscriptSegments[:1]
				conversationCtx.Manifest.Selected["call_transcript_segments"] = len(conversationCtx.TranscriptSegments)
			}
		}},
		{"messages", func() {
			// Guard: never remove the last message when present.
			if len(conversationCtx.Messages) > 1 {
				conversationCtx.Messages = conversationCtx.Messages[:1]
				conversationCtx.Manifest.Selected["messages"] = len(conversationCtx.Messages)
			}
		}},
		{"memories", func() {
			filtered := make([]models.AgentMemory, 0, len(conversationCtx.Memories))
			for _, mem := range conversationCtx.Memories {
				if mem.Importance >= 80 {
					filtered = append(filtered, mem)
				}
			}
			if len(filtered) < len(conversationCtx.Memories) {
				conversationCtx.Memories = filtered
				conversationCtx.Manifest.Selected["memories"] = len(conversationCtx.Memories)
			}
		}},
		{"notes", func() {
			if len(conversationCtx.Notes) > 1 {
				conversationCtx.Notes = conversationCtx.Notes[:1]
				conversationCtx.Manifest.Selected["notes"] = len(conversationCtx.Notes)
			}
		}},
		{"context_chunks", func() {
			if len(conversationCtx.ContextChunks) > 1 {
				filtered := make([]RetrievedContextChunk, 0, len(conversationCtx.ContextChunks))
				for _, chunk := range conversationCtx.ContextChunks {
					if chunk.Score >= 5 {
						filtered = append(filtered, chunk)
					}
				}
				if len(filtered) < len(conversationCtx.ContextChunks) {
					conversationCtx.ContextChunks = filtered
					conversationCtx.Manifest.Selected["chunks"] = len(conversationCtx.ContextChunks)
				}
			}
		}},
	}

	for _, step := range trimSteps {
		if conversationCtx.Manifest.SerializedBytes <= budget.MaxBytes && estimatedTokens <= budget.MaxEstimatedTokens {
			break
		}
		step.trim()
		conversationCtx.Manifest.recordTruncation(step.name)

		// Re-estimate tokens from the reduced collection (cheap heuristic).
		estimatedTokens = estimateContextTokens(conversationCtx)
		conversationCtx.Manifest.EstimatedTokens = estimatedTokens
	}

	// Final serialization to confirm actual byte count after all trimming.
	request = buildAgentRuntimeRequest(models.AgentRun{
		OrganizationID: conversationCtx.Conversation.OrganizationID,
		UserID:         0,
		ConversationID: conversationCtx.Conversation.ID,
	}, "budget_check", conversationCtx)
	raw, err = json.Marshal(request)
	if err == nil {
		conversationCtx.Manifest.SerializedBytes = len(raw)
	}
}
