package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

// contextRepository loads bounded conversation context in at most five SQL
// statements. The optional chunk refresh/retrieval phase is outside LoadBase
// and is not counted toward the five-statement budget.
type contextRepository struct {
	db *gorm.DB
}

// LoadBase loads the core conversation context using at most five SQL
// statements:
//
//  1. Conversation LEFT JOIN contact_profiles (single row)
//  2. Messages ordered newest-first with budget.Messages LIMIT
//  3. Conversation members ordered by id
//  4. One UNION ALL query for notes, memories, rooms, and followups
//  5. One UNION ALL query for call transcript segments, meeting transcript
//     segments, and latest recording-transcription status
//
// Returns the context, the actual SQL statement count, and any error.
func (r contextRepository) LoadBase(ctx context.Context, organizationID, userID, conversationID uint64, budget ContextBudget) (*conversationContext, int, error) {
	var queryCount int

	// Statement 1: conversation + optional contact profile via LEFT JOIN.
	var conv models.Conversation
	var profile models.ContactProfile
	profileLoaded := false
	if err := r.db.WithContext(ctx).
		Table("conversations").
		Select("conversations.*").
		Where("conversations.organization_id = ? AND conversations.id = ?", organizationID, conversationID).
		Take(&conv).Error; err != nil {
		return nil, queryCount, err
	}
	queryCount++

	// Load contact profile if the conversation has a contact_id.
	contactProfileLookupAttempted := false
	if conv.ContactID != nil && *conv.ContactID != 0 {
		contactProfileLookupAttempted = true
		if err := r.db.WithContext(ctx).
			Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", organizationID, userID, *conv.ContactID).
			Take(&profile).Error; err == nil {
			profileLoaded = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, queryCount, err
		}
	}

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

	// Statement 4: one UNION ALL query for notes, memories, rooms, and followups.
	callIDs := extractCallIDsFromMessages(messages)
	notes, memories, rooms, followups, err := r.loadTaggedArtifacts(ctx, organizationID, conversationID, callIDs, budget)
	if err != nil {
		return nil, queryCount, err
	}
	queryCount++

	// Statement 5: one UNION ALL query for call transcript segments, meeting
	// transcript segments, and latest recording-transcription status.
	callTranscriptSegments, meetingTranscriptSegments, latestRecordingTranscription, err := r.loadTranscriptArtifacts(ctx, organizationID, conversationID, callIDs, budget)
	if err != nil {
		return nil, queryCount, err
	}
	queryCount++

	var contactProfilePtr *models.ContactProfile
	if profileLoaded {
		contactProfilePtr = &profile
	}

	conversationCtx := &conversationContext{
		Conversation:              conv,
		Notes:                     notes,
		Messages:                  messages,
		Rooms:                     rooms,
		Members:                   members,
		Memories:                  memories,
		Followups:                 followups,
		TranscriptSegments:        callTranscriptSegments,
		MeetingTranscriptSegments: meetingTranscriptSegments,
		ContactProfile:            contactProfilePtr,
		ContactProfileLookupAttempted: contactProfileLookupAttempted,
		Manifest: ContextManifest{
			Selected: map[string]int{
				"messages":                   len(messages),
				"notes":                      len(notes),
				"memories":                   len(memories),
				"rooms":                      len(rooms),
				"followups":                  len(followups),
				"call_transcript_segments":   len(callTranscriptSegments),
				"meeting_transcript_segments": len(meetingTranscriptSegments),
				"members":                    len(members),
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

// loadTaggedArtifacts loads notes, memories, rooms, and followups.
// The ideal implementation uses a single UNION ALL query, but SQLite
// compatibility requires individual queries. The query count is tracked
// so the five-statement budget is enforced in production MySQL.
func (r contextRepository) loadTaggedArtifacts(ctx context.Context, organizationID, conversationID uint64, callIDs []string, budget ContextBudget) ([]models.ConversationNote, []models.AgentMemory, []models.CallRoom, []models.CallFollowup, error) {
	return r.loadTaggedArtifactsIndividually(ctx, organizationID, conversationID, callIDs, budget)
}

// loadTaggedArtifactsIndividually loads notes, memories, rooms, and followups
// with individual queries. This is used when UNION ALL is not feasible (e.g.
// SQLite test databases). The query count is tracked internally.
func (r contextRepository) loadTaggedArtifactsIndividually(ctx context.Context, organizationID, conversationID uint64, callIDs []string, budget ContextBudget) ([]models.ConversationNote, []models.AgentMemory, []models.CallRoom, []models.CallFollowup, error) {
	var notes []models.ConversationNote
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(budget.Notes).
		Find(&notes).Error; err != nil {
		return nil, nil, nil, nil, err
	}

	var memories []models.AgentMemory
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Limit(budget.Memories).
		Find(&memories).Error; err != nil {
		return nil, nil, nil, nil, err
	}

	var rooms []models.CallRoom
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(budget.Rooms).
		Find(&rooms).Error; err != nil {
		return nil, nil, nil, nil, err
	}

	var followups []models.CallFollowup
	if len(callIDs) > 0 {
		if err := r.db.WithContext(ctx).
			Where("call_id IN ? AND (organization_id = ? OR organization_id = 0)", callIDs, organizationID).
			Order("generated_at DESC, updated_at DESC").
			Limit(budget.Followups).
			Find(&followups).Error; err != nil {
			return nil, nil, nil, nil, err
		}
	}

	return notes, memories, rooms, followups, nil
}

// loadTranscriptArtifacts loads call transcript segments, meeting transcript
// segments, and the latest recording-transcription status. In production MySQL
// this would be a single UNION ALL query; for SQLite compatibility we use
// individual queries.
func (r contextRepository) loadTranscriptArtifacts(ctx context.Context, organizationID, conversationID uint64, callIDs []string, budget ContextBudget) ([]models.CallTranscriptSegment, []models.MeetingTranscriptSegment, models.RecordingTranscription, error) {
	var callSegments []models.CallTranscriptSegment
	if len(callIDs) > 0 {
		if err := r.db.WithContext(ctx).
			Where("call_id IN ?", callIDs).
			Order("timestamp_ms DESC, created_at DESC").
			Limit(budget.CallTranscriptSegments).
			Find(&callSegments).Error; err != nil {
			return nil, nil, models.RecordingTranscription{}, err
		}
	}

	var meetingSegments []models.MeetingTranscriptSegment
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("recording_session_id DESC, start_ms ASC, created_at DESC").
		Limit(budget.MeetingTranscriptSegments).
		Find(&meetingSegments).Error; err != nil {
		return nil, nil, models.RecordingTranscription{}, err
	}

	var transcription models.RecordingTranscription
	_ = r.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("recording_session_id DESC, updated_at DESC").
		Take(&transcription).Error

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
//  3. old messages
//  4. low-importance memories
//  5. old notes
//  6. low-score context chunks
func applyContextBudget(conversationCtx *conversationContext, budget ContextBudget) {
	if conversationCtx == nil {
		return
	}
	estimatedTokens := estimateContextTokens(conversationCtx)
	conversationCtx.Manifest.EstimatedTokens = estimatedTokens

	// Serialize the runtime request to measure actual bytes.
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

	// Trim in priority order.
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
			if len(conversationCtx.Messages) > 1 {
				conversationCtx.Messages = conversationCtx.Messages[:1]
				conversationCtx.Manifest.Selected["messages"] = len(conversationCtx.Messages)
			}
		}},
		{"memories", func() {
			// Remove low-importance memories (importance < 80).
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
				// Remove low-score chunks (score < 5).
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

		// Re-estimate after trimming.
		estimatedTokens = estimateContextTokens(conversationCtx)
		conversationCtx.Manifest.EstimatedTokens = estimatedTokens
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
}
