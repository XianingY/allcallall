package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// workflowResultCollections holds batch-loaded child collections keyed by run ID.
// Each slice is ordered id ASC (matching the single-run contract) and already
// truncated to workflowResultMaxRows per run.
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
// in a fixed number of SQL queries (one per table). Each query uses UNION ALL
// with one subquery per run, each independently limited to workflowResultMaxRows,
// so no single heavy run can starve others of their row budget.
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

// loadBatchedCollection loads one child table for all runIDs in a single SQL
// statement composed of per-run subqueries joined by UNION ALL. Each subquery
// independently applies LIMIT workflowResultMaxRows, so every run is bounded
// at the database level regardless of how many rows other runs consume.
// Rows are then grouped by run in Go, truncated per run if needed, and
// reversed to id ASC order.
func loadBatchedCollection[T any](ctx context.Context, db *gorm.DB, runIDs []uint64, dst *map[uint64][]T, truncated map[uint64]bool) error {
	if len(runIDs) == 0 {
		return nil
	}

	tableName := childTableName[T]()
	var sqlBuilder strings.Builder
	args := make([]any, 0, len(runIDs)*2)

	for i, runID := range runIDs {
		if i > 0 {
			sqlBuilder.WriteString(" UNION ALL ")
		}
		fmt.Fprintf(&sqlBuilder,
			"SELECT * FROM (SELECT * FROM %s WHERE workflow_run_id = ? ORDER BY id DESC LIMIT ?) AS sub_%d",
			tableName, i)
		args = append(args, runID, workflowResultMaxRows)
	}

	var rows []T
	if err := db.WithContext(ctx).Raw(sqlBuilder.String(), args...).Scan(&rows).Error; err != nil {
		return err
	}

	// Group by run ID, detecting truncation per run.
	type runBucket struct {
		rows []T
	}
	buckets := make(map[uint64]*runBucket, len(runIDs))
	for i := range rows {
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

// childTableName returns the SQL table name for a child collection model type.
// It calls the model's TableName() method through a type switch, avoiding
// reflection and keeping the name in sync with the model definition.
func childTableName[T any]() string {
	var zero T
	type tabler interface{ TableName() string }
	if t, ok := any(zero).(tabler); ok {
		return t.TableName()
	}
	panic(fmt.Sprintf("childTableName: type %T does not implement TableName()", zero))
}

// extractWorkflowRunID returns the WorkflowRunID field from any of the six
// child model types using a type switch (no reflection).
func extractWorkflowRunID(row any) uint64 {
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
