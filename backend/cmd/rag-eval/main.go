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
	fixturePath := flag.String("fixture", agent.DefaultRAGEvalFixture, "path to RAG eval cases JSON")
	flag.Parse()

	cases, err := agent.LoadRAGEvalCases(*fixturePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load rag eval cases failed: %v\n", err)
		os.Exit(2)
	}
	report, err := agent.RunRAGEval(context.Background(), cases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run rag eval failed: %v\n", err)
		os.Exit(2)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "write rag eval report failed: %v\n", err)
		os.Exit(2)
	}
	if report.Failed > 0 {
		os.Exit(1)
	}
}
