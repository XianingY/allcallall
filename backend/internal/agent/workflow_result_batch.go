package agent

import (
	"context"
	"encoding/json"
	"strings"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// workflowResultCollections holds batch-loaded child collections keyed by run ID.
// Each slice is ordered id ASC (matching the single-run loadWorkflowCollection
// contract) and already truncated to workflowResultMaxRows per run.
type workflowResultCollections struct {
	Tasks     map[uint64][]models.WorkflowTask
	Messages  map[uint64][]models.AgentMessage
	Approvals map[uint64][]models.ToolApproval
	History   map[uint64][]models.WorkflowHistoryEvent
	Signals   map[uint64][]models.WorkflowSignal
	Timers    map[uint64][]models.WorkflowTimer
	Truncated map[uint64]bool
}

// loadWorkflowResultCollections loads all six child tables for the given run IDs
// in a fixed number of SQL queries (one per table). For each table the rows are
// ordered by workflow_run_id ASC, id DESC, then grouped by run, truncated to
// workflowResultMaxRows per run, and finally reversed to id ASC order.
func (s *Service) loadWorkflowResultCollections(ctx context.Context, runIDs []uint64) (workflowResultCollections, error) {
	collections := workflowResultCollections{
		Tasks:     make(map[uint64][]models.WorkflowTask, len(runIDs)),
		Messages:  make(map[uint64][]models.AgentMessage, len(runIDs)),
		Approvals: make(map[uint64][]models.ToolApproval, len(runIDs)),
		History:   make(map[uint64][]models.WorkflowHistoryEvent, len(runIDs)),
		Signals:   make(map[uint64][]models.WorkflowSignal, len(runIDs)),
		Timers:    make(map[uint64][]models.WorkflowTimer, len(runIDs)),
		Truncated: make(map[uint64]bool, len(runIDs)),
	}

	if err := loadBatchedCollection(ctx, s.db, runIDs, &collections.Tasks, collections.Truncated); err != nil {
		return collections, err
	}
	if err := loadBatchedCollection(ctx, s.db, runIDs, &collections.Messages, collections.Truncated); err != nil {
		return collections, err
	}
	if err := loadBatchedCollection(ctx, s.db, runIDs, &collections.Approvals, collections.Truncated); err != nil {
		return collections, err
	}
	if err := loadBatchedCollection(ctx, s.db, runIDs, &collections.History, collections.Truncated); err != nil {
		return collections, err
	}
	if err := loadBatchedCollection(ctx, s.db, runIDs, &collections.Signals, collections.Truncated); err != nil {
		return collections, err
	}
	if err := loadBatchedCollection(ctx, s.db, runIDs, &collections.Timers, collections.Truncated); err != nil {
		return collections, err
	}

	return collections, nil
}

// loadBatchedCollection loads one child table for all runIDs in a single query,
// groups by run, truncates per run, and reverses to id ASC order.
func loadBatchedCollection[T any](ctx context.Context, db *gorm.DB, runIDs []uint64, dst *map[uint64][]T, truncated map[uint64]bool) error {
	var rows []T
	if err := db.WithContext(ctx).
		Where("workflow_run_id IN ?", runIDs).
		Order("workflow_run_id ASC, id DESC").
		Find(&rows).Error; err != nil {
		return err
	}

	// Group by run ID, counting per-run to detect truncation.
	type runBucket struct {
		rows []T
	}
	buckets := make(map[uint64]*runBucket, len(runIDs))
	for i := range rows {
		// Extract WorkflowRunID from the generic row. Since T always has a
		// WorkflowRunID uint64 field, we use a small interface-based accessor.
		runID := extractWorkflowRunID(&rows[i])
		b, ok := buckets[runID]
		if !ok {
			b = &runBucket{}
			buckets[runID] = b
		}
		b.rows = append(b.rows, rows[i])
	}

	for runID, b := range buckets {
		if len(b.rows) >= workflowResultMaxRows {
			truncated[runID] = true
			// Keep only the most recent workflowResultMaxRows rows.
			// Since rows are in id DESC order, the first workflowResultMaxRows
			// entries are the most recent.
			b.rows = b.rows[:workflowResultMaxRows]
		}
		// Reverse to id ASC order to match the single-run contract.
		n := len(b.rows)
		for i := 0; i < n/2; i++ {
			b.rows[i], b.rows[n-1-i] = b.rows[n-1-i], b.rows[i]
		}
		(*dst)[runID] = b.rows
	}

	// Ensure every runID has an entry (even if empty slice) so projectWorkflowResult
	// never has to check for map absence.
	for _, id := range runIDs {
	 if _, ok := (*dst)[id]; !ok {
			(*dst)[id] = []T{}
		}
	}

	return nil
}

// workflowRunIDHolder is used to extract WorkflowRunID from any model struct
// that contains a WorkflowRunID uint64 field.
type workflowRunIDHolder interface {
	GetWorkflowRunID() uint64
}

// extractWorkflowRunID uses GORM's reflection to pull the WorkflowRunID field
// from any model struct that has it.
func extractWorkflowRunID(row any) uint64 {
	// All six child models have a WorkflowRunID field. We use a small
	// type-switch to avoid reflection overhead in hot paths.
	switch v := row.(type) {
	case *models.WorkflowTask:
		return v.WorkflowRunID
	case *models.AgentMessage:
		return v.WorkflowRunID
	case *models.ToolApproval:
		return v.WorkflowRunID
	case *models.WorkflowHistoryEvent:
		return v.WorkflowRunID
	case *models.WorkflowSignal:
		return v.WorkflowRunID
	case *models.WorkflowTimer:
		return v.WorkflowRunID
	default:
		return 0
	}
}

// projectWorkflowResult assembles a WorkflowResult from a single run and its
// pre-loaded collections. This is the shared projector used by both the
// single-run buildWorkflowResult and the batch buildWorkflowResults.
func projectWorkflowResult(run models.WorkflowRun, collections workflowResultCollections) WorkflowResult {
	var citations []Citation
	if strings.TrimSpace(run.CitationsJSON) != "" {
		_ = json.Unmarshal([]byte(run.CitationsJSON), &citations)
	}

	tasks := collections.Tasks[run.ID]
	messages := collections.Messages[run.ID]
	approvals := collections.Approvals[run.ID]
	history := collections.History[run.ID]
	signals := collections.Signals[run.ID]
	timers := collections.Timers[run.ID]

	// Ensure non-nil slices for JSON serialization consistency.
	if tasks == nil {
		tasks = []models.WorkflowTask{}
	}
	if messages == nil {
		messages = []models.AgentMessage{}
	}
	if approvals == nil {
		approvals = []models.ToolApproval{}
	}
	if history == nil {
		history = []models.WorkflowHistoryEvent{}
	}
	if signals == nil {
		signals = []models.WorkflowSignal{}
	}
	if timers == nil {
		timers = []models.WorkflowTimer{}
	}

	return WorkflowResult{
		Run:         run,
		Tasks:       tasks,
		Messages:    messages,
		Approvals:   approvals,
		History:     history,
		Signals:     signals,
		Timers:      timers,
		Citations:   citations,
		ActionItems: decodeStringSlice(run.ActionItemsJSON),
		RiskFlags:   decodeStringSlice(run.RiskFlagsJSON),
		Truncated:   collections.Truncated[run.ID],
	}
}

// buildWorkflowResults builds WorkflowResults for multiple runs using batched
// collection loading. It issues a fixed number of SQL queries (one per child
// table) regardless of the number of runs.
func (s *Service) buildWorkflowResults(ctx context.Context, runs []models.WorkflowRun) ([]WorkflowResult, error) {
	if len(runs) == 0 {
		return []WorkflowResult{}, nil
	}
	runIDs := make([]uint64, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}
	collections, err := s.loadWorkflowResultCollections(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	results := make([]WorkflowResult, 0, len(runs))
	for _, run := range runs {
		results = append(results, projectWorkflowResult(run, collections))
	}
	return results, nil
}
