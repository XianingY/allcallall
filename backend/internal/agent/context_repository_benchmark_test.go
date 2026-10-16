package agent

import (
	"context"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

// benchContextBudget 显式固定预算，避免环境变量使基准结果不可比。
// 数值与 ContextBudgetFromEnv 的默认档一致。
func benchContextBudget() ContextBudget {
	return ContextBudget{
		Messages:                  50,
		Notes:                     20,
		Memories:                  10,
		Rooms:                     3,
		Followups:                 10,
		CallTranscriptSegments:    40,
		MeetingTranscriptSegments: 80,
		Chunks:                    8,
		MaxBytes:                  64 * 1024,
		MaxEstimatedTokens:        8000,
	}
}

// BenchmarkAgentContextLoadBase measures the context assembly seam every
// agent run goes through: the bounded five-statement context query plus
// payload decryption. SQLite keeps the benchmark hermetic; compare numbers
// only against the same benchmark on the same machine.
func BenchmarkAgentContextLoadBase(b *testing.B) {
	repo, db := newContextRepositoryTestEnv(b)
	conv, userID := seedContextConversation(b, db)
	seedAllContextTypes(b, db, conv, userID)

	// 顶到预算上限，量 worst-case 选取成本而不是近乎空表。
	for i := 0; i < 60; i++ {
		if err := db.Create(&models.Message{
			OrganizationID: conv.OrganizationID,
			ConversationID: conv.ID,
			SenderID:       userID,
			Body:           "Context benchmark message body with enough text to exercise estimation.",
		}).Error; err != nil {
			b.Fatalf("create message: %v", err)
		}
	}

	ctx := context.Background()
	budget := benchContextBudget()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loaded, statements, err := repo.LoadBase(ctx, conv.OrganizationID, userID, conv.ID, budget)
		if err != nil {
			b.Fatalf("LoadBase failed: %v", err)
		}
		if loaded == nil || statements > 5 {
			b.Fatalf("unexpected LoadBase result: statements=%d", statements)
		}
	}
}

// BenchmarkAgentContextEstimateTokens measures the pure-CPU token estimator
// on a fully loaded context, the other half of the per-run context cost.
func BenchmarkAgentContextEstimateTokens(b *testing.B) {
	repo, db := newContextRepositoryTestEnv(b)
	conv, userID := seedContextConversation(b, db)
	seedAllContextTypes(b, db, conv, userID)

	loaded, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, benchContextBudget())
	if err != nil {
		b.Fatalf("LoadBase failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if estimateContextTokens(loaded) <= 0 {
			b.Fatalf("expected positive token estimate")
		}
	}
}
