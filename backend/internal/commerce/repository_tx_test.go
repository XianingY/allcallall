package commerce

import (
	"testing"

	"gorm.io/gorm"
)

// TestWithTxBindsStatementsToTheTransaction guards a bug that silently dropped
// paid purchases: HandleRevenueCatWebhook wrapped its work in
// RunInTransaction but every call inside the closure went through s.repo,
// which takes its own connection from the pool. The transaction therefore
// contained no statements, so a failure after the event row was inserted
// rolled back nothing, and the leftover row made every retry answer
// "already processed" - the purchase was never applied.
func TestWithTxBindsStatementsToTheTransaction(t *testing.T) {
	base := NewRepository(&gorm.DB{})
	tx := &gorm.DB{}

	bound := base.WithTx(tx)

	if bound.db != tx {
		t.Fatal("WithTx must make statements run on the transaction")
	}
	if bound == base {
		t.Fatal("WithTx must return a copy, not the same repository")
	}
	if base.db == tx {
		t.Fatal("WithTx must not mutate the original repository")
	}
}
