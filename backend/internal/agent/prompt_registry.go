package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

const CurrentWorkflowPromptVersion = "workflow_prompt_v1"

type promptDescriptor struct {
	Name     string
	Template string
}

func workflowPromptDescriptors() []promptDescriptor {
	return []promptDescriptor{
		{Name: models.WorkflowTaskCollectContext, Template: "Collect conversation, note, room, memory, and RAG context."},
		{Name: models.WorkflowTaskDecompose, Template: "Decompose the workflow goal into parallel role tasks."},
		{Name: models.WorkflowTaskSearcher, Template: "Search for grounded evidence and citation candidates."},
		{Name: models.WorkflowTaskSummarizer, Template: "Summarize the conversation using grounded context."},
		{Name: models.WorkflowTaskRiskAnalyst, Template: "Identify risks, blockers, and escalation points."},
		{Name: models.WorkflowTaskMerge, Template: "Merge role outputs into one workflow result."},
		{Name: models.WorkflowTaskProposeTools, Template: "Propose tool actions within policy constraints."},
	}
}

func workflowPromptHash(name, template string) string {
	sum := sha256.Sum256([]byte(name + "\n" + template))
	return hex.EncodeToString(sum[:])
}

func (s *Service) ensureWorkflowMetadataRegistered(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range workflowPromptDescriptors() {
			row := models.AgentPromptVersion{
				Name:        item.Name,
				Version:     CurrentWorkflowPromptVersion,
				ContentHash: workflowPromptHash(item.Name, item.Template),
				Template:    item.Template,
			}
			if err := tx.Where("name = ? AND version = ?", row.Name, row.Version).Attrs(row).FirstOrCreate(&row).Error; err != nil {
				return err
			}
		}
		for _, tool := range RegisteredTools() {
			schemaJSON, _ := json.Marshal(map[string]any{
				"input_schema":  tool.InputSchema,
				"output_schema": tool.OutputSchema,
			})
			row := models.ToolSchemaVersion{
				Name:       tool.Name,
				Version:    tool.Version,
				SchemaHash: tool.SchemaHash,
				SchemaJSON: string(schemaJSON),
			}
			if err := tx.Where("name = ? AND version = ?", row.Name, row.Version).Attrs(row).FirstOrCreate(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
