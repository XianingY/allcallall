package collaboration

import (
	"reflect"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestBuildConversationUpdatePlan(t *testing.T) {
	assigneeID := uint64(7)
	contactID := uint64(42)
	base := models.Conversation{
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityNormal,
		AssigneeUserID: &assigneeID,
	}

	tests := []struct {
		name              string
		input             UpdateConversationInput
		wantUpdates       []string
		wantEvents        int
		wantValidateID    *uint64
		wantAssigneeClear bool
		wantContactClear  bool
	}{
		{
			name: "status and priority change",
			input: UpdateConversationInput{
				Status:   ptrString(models.ConversationStatusPending),
				Priority: ptrString(models.ConversationPriorityHigh),
			},
			wantUpdates: []string{"status", "priority"},
			wantEvents:  2,
		},
		{
			name: "same assignee still validates membership without writing",
			input: UpdateConversationInput{
				AssigneeUserID: ptrUint64(assigneeID),
			},
			wantValidateID: &assigneeID,
		},
		{
			name: "clear assignee",
			input: UpdateConversationInput{
				AssigneeUserID: ptrUint64(0),
			},
			wantUpdates:       []string{"assignee_user_id"},
			wantEvents:        1,
			wantAssigneeClear: true,
		},
		{
			name: "bind contact",
			input: UpdateConversationInput{
				ContactID: &contactID,
			},
			wantUpdates: []string{"contact_id"},
		},
		{
			name: "clear contact",
			input: UpdateConversationInput{
				ContactID: ptrUint64(0),
			},
			wantUpdates:      []string{"contact_id"},
			wantContactClear: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := buildConversationUpdatePlan(base, tt.input)
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}
			for _, key := range tt.wantUpdates {
				if _, ok := plan.Updates[key]; !ok {
					t.Fatalf("expected update key %s in %#v", key, plan.Updates)
				}
			}
			if len(plan.SystemEvents) != tt.wantEvents {
				t.Fatalf("expected %d system events, got %d", tt.wantEvents, len(plan.SystemEvents))
			}
			if tt.wantValidateID == nil {
				if plan.AssigneeUserIDToValidate != nil {
					t.Fatalf("expected no assignee validation, got %d", *plan.AssigneeUserIDToValidate)
				}
			} else if plan.AssigneeUserIDToValidate == nil || *plan.AssigneeUserIDToValidate != *tt.wantValidateID {
				t.Fatalf("expected assignee validation for %d, got %#v", *tt.wantValidateID, plan.AssigneeUserIDToValidate)
			}
			if tt.wantAssigneeClear && !isNilUpdateValue(plan.Updates["assignee_user_id"]) {
				t.Fatalf("expected assignee update to nil, got %#v", plan.Updates["assignee_user_id"])
			}
			if tt.wantContactClear && !isNilUpdateValue(plan.Updates["contact_id"]) {
				t.Fatalf("expected contact update to nil, got %#v", plan.Updates["contact_id"])
			}
		})
	}
}

func isNilUpdateValue(value any) bool {
	if value == nil {
		return true
	}
	item := reflect.ValueOf(value)
	switch item.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return item.IsNil()
	default:
		return false
	}
}

func TestBuildConversationUpdatePlanRejectsInvalidStatus(t *testing.T) {
	_, err := buildConversationUpdatePlan(models.Conversation{}, UpdateConversationInput{
		Status: ptrString("not-a-status"),
	})
	if err == nil {
		t.Fatal("expected invalid status to fail")
	}
}

func TestBuildConversationPatchChanges(t *testing.T) {
	assigneeID := uint64(9)
	contactID := uint64(10)
	summary := ConversationSummary{
		Conversation: models.Conversation{
			Status:         models.ConversationStatusResolved,
			Priority:       models.ConversationPriorityUrgent,
			AssigneeUserID: &assigneeID,
			ContactID:      &contactID,
		},
		AssigneeEmail:       "agent@example.com",
		AssigneeDisplayName: "Agent",
	}

	changes := buildConversationPatchChanges(summary, []string{"status", "assignee_user_id", "assignee_user_id", "contact_id"})
	if changes["status"] != models.ConversationStatusResolved {
		t.Fatalf("expected status patch, got %#v", changes["status"])
	}
	if changes["assignee_user_id"] != summary.AssigneeUserID {
		t.Fatalf("expected assignee id patch, got %#v", changes["assignee_user_id"])
	}
	if changes["assignee_email"] != "agent@example.com" {
		t.Fatalf("expected assignee email patch, got %#v", changes["assignee_email"])
	}
	if changes["contact_id"] != summary.ContactID {
		t.Fatalf("expected contact id patch, got %#v", changes["contact_id"])
	}
}
