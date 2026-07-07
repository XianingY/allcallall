package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/allcallall/backend/internal/evals"
)

func main() {
	evalType := flag.String("type", "task", "eval type: task, demo, rag, workflow")
	provider := flag.String("provider", "rules", "agent provider")
	fixture := flag.String("fixture", evals.DefaultTaskEvalFixture, "path to fixture")
	flag.Parse()

	fmt.Printf("Starting %s eval with provider %s...\n", *evalType, *provider)

	switch *evalType {
	case "task":
		cases, err := evals.LoadAgentTaskEvalCases(*fixture)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load cases: %v\n", err)
			os.Exit(1)
		}
		report, err := evals.RunAgentTaskEvalWithOptions(context.Background(), cases, evals.AgentTaskEvalOptions{
			Runtime: *provider,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to run eval: %v\n", err)
			os.Exit(1)
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(report)
	default:
		fmt.Printf("Eval type %s is configured via allcallallctl\n", *evalType)
	}
}
