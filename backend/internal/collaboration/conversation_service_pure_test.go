package collaboration

import (
	"testing"

	"github.com/allcallall/backend/internal/models"
)

// TestCollectSummaryUserIDs verifies the pure helper that decides which users
// to fetch in a single batch for ListConversations. It must only request peer
// lookups for direct conversations lacking a title, and must deduplicate
// assignee ids across conversations.
func TestCollectSummaryUserIDs(t *testing.T) {
	directNoTitle := models.Conversation{ID: 1, Type: models.ConversationTypeDirect, Title: ""}
	directWithTitle := models.Conversation{ID: 2, Type: models.ConversationTypeDirect, Title: "Team"}
	channel := models.Conversation{ID: 3, Type: models.ConversationTypeChannel, Title: ""}
	assigneeA := uint64(10)
	assigneeB := uint64(11)
	withAssignee := models.Conversation{ID: 4, Type: models.ConversationTypeDirect, AssigneeUserID: &assigneeA}
	duplicateAssignee := models.Conversation{ID: 5, Type: models.ConversationTypeDirect, AssigneeUserID: &assigneeA}
	secondAssignee := models.Conversation{ID: 6, Type: models.ConversationTypeDirect, AssigneeUserID: &assigneeB}

	convs := []models.Conversation{
		directNoTitle,
		directWithTitle,
		channel,
		withAssignee,
		duplicateAssignee,
		secondAssignee,
	}

	directConvIDs, assigneeIDs := collectSummaryUserIDs(convs, 99)

	// Every direct conversation without a title needs a peer lookup, including
	// those that also carry an assignee (their title is still the peer name).
	if len(directConvIDs) != 4 {
		t.Fatalf("expected 4 direct peer lookups, got %v", directConvIDs)
	}
	expectedDirect := map[uint64]bool{1: true, 4: true, 5: true, 6: true}
	for _, id := range directConvIDs {
		if !expectedDirect[id] {
			t.Fatalf("unexpected direct conv id %d in %v", id, directConvIDs)
		}
	}

	// Assignee ids are deduplicated (10 appears twice) and preserve both values.
	if len(assigneeIDs) != 2 {
		t.Fatalf("expected 2 unique assignee ids, got %v", assigneeIDs)
	}
	seen := map[uint64]bool{}
	for _, id := range assigneeIDs {
		if seen[id] {
			t.Fatalf("duplicate assignee id %d in %v", id, assigneeIDs)
		}
		seen[id] = true
	}
	if !seen[10] || !seen[11] {
		t.Fatalf("expected assignee ids 10 and 11, got %v", assigneeIDs)
	}
}
