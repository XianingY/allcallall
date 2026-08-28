// Command opsaudit runs the Phase 4 commercial-operations jobs from the
// command line or CI: the annual compliance self-audit and the growth &
// retention analytics report. It is intentionally outside the request path.
//
// Examples:
//
//	# Compliance self-audit (no DB required, env-driven)
//	go run ./cmd/opsaudit -report compliance
//
//	# Growth report against production read replica
//	go run ./cmd/opsaudit -report growth \
//	  -dsn "mysql://app:pass@tcp(db:3306)/allcallall?readOnly=true"
//
//	# Both, written to a file for archival
//	go run ./cmd/opsaudit -report all -dsn "sqlite:///snapshot.db" -out audit.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/opsjobs"
)

func main() {
	report := flag.String("report", "all", "report to run: growth|compliance|all")
	dsn := flag.String("dsn", "", "DB DSN: mysql://user:pass@tcp(host:3306)/db or sqlite:///path.db")
	out := flag.String("out", "", "output file (default stdout)")
	months := flag.Int("months", 6, "growth window in months")
	flag.Parse()

	wantGrowth := *report == "growth" || *report == "all"
	if wantGrowth {
		if *dsn == "" {
			fmt.Fprintln(os.Stderr, "growth report requires -dsn")
			os.Exit(2)
		}
	}

	payload := map[string]interface{}{}

	if *report == "compliance" || *report == "all" {
		audit := opsjobs.RunAnnualAudit()
		payload["compliance"] = audit
	}

	if *report == "plan" || *report == "all" {
		plan := opsjobs.BuildQuarterlyPlan(time.Now())
		payload["pentest_plan"] = plan
	}

	if wantGrowth {
		db, err := openDB(*dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open db: %v\n", err)
			os.Exit(1)
		}
		rep, err := opsjobs.NewGrowthRetentionAnalyzer(db).Report(context.Background(), *months)
		if err != nil {
			fmt.Fprintf(os.Stderr, "growth report: %v\n", err)
			os.Exit(1)
		}
		payload["growth"] = rep
	}

	var w *os.File = os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create out: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}

func openDB(dsn string) (*gorm.DB, error) {
	switch {
	case len(dsn) > 9 && dsn[:9] == "sqlite:///":
		return gorm.Open(sqlite.Open(dsn[9:]), &gorm.Config{})
	case len(dsn) > 8 && dsn[:8] == "sqlite:/":
		return gorm.Open(sqlite.Open(dsn[8:]), &gorm.Config{})
	case len(dsn) > 6 && dsn[:6] == "mysql:":
		conn := dsn[6:]
		if len(conn) >= 2 && conn[:2] == "//" {
			conn = conn[2:]
		}
		return gorm.Open(mysql.Open(conn), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported dsn scheme: %s", dsn)
	}
}
