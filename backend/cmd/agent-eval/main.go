package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/allcallall/backend/internal/agent"
)

func main() {
	providerName := flag.String("provider", "rules", "agent planner provider: rules, mock_llm, openai_compatible")
	fixturePath := flag.String("fixture", "./internal/agent/testdata/eval_cases.json", "path to eval cases JSON")
	flag.Parse()

	planner, err := agent.NewPlanner(*providerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create planner failed: %v\n", err)
		os.Exit(2)
	}
	cases, err := agent.LoadEvalCases(*fixturePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load eval cases failed: %v\n", err)
		os.Exit(2)
	}
	report, err := agent.RunPlannerEval(context.Background(), planner, cases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run eval failed: %v\n", err)
		os.Exit(2)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "write eval report failed: %v\n", err)
		os.Exit(2)
	}
	if report.Failed > 0 {
		os.Exit(1)
	}
}
