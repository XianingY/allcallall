package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/allcallall/backend/internal/interviewbench"
)

func main() {
	cfg := parseFlags()
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "interview bench failed: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() interviewbench.Config {
	cfg := interviewbench.Config{}
	flag.IntVar(&cfg.Conversations, "conversations", 25, "number of seeded conversations and Agent runs")
	flag.IntVar(&cfg.BatchSize, "batch-size", 50, "outbox processor batch size")
	flag.StringVar(&cfg.Provider, "provider", "rules", "agent provider: rules, mock_llm, or openai_compatible")
	flag.BoolVar(&cfg.KeepDB, "keep-db", false, "keep the temporary sqlite database and print its path")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg interviewbench.Config, writer io.Writer) error {
	output, err := interviewbench.Run(ctx, cfg)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
