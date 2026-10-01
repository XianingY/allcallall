package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseBody parses a function body and returns it, so the scanner can be
// exercised on source text rather than on the repository itself.
func parseBody(t *testing.T, body string) *ast.BlockStmt {
	t.Helper()
	src := "package p\n\nfunc f(db *gorm.DB) {\n" + body + "\n}\n"
	file, err := parser.ParseFile(token.NewFileSet(), "probe.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Body == nil {
		t.Fatal("expected a function body")
	}
	return fn.Body
}

// The regression this guards: the scanner used to ask "does .Limit( appear
// anywhere in this function", so one paginated query excused every other
// unpaginated one in the same function - and list functions routinely query
// more than one table.
func TestUnpaginatedFindIsNotExcusedByASiblingQuery(t *testing.T) {
	body := parseBody(t, `
		var all []row
		db.Find(&all)

		var some []row
		db.Limit(10).Find(&some)
	`)

	hasFind, hasUnpagedFind, _ := scanBody(body)
	if !hasFind {
		t.Fatal("expected the scanner to see a result-set Find")
	}
	if !hasUnpagedFind {
		t.Fatal("a Find without page control must be reported even when another Find in the same function is paginated")
	}
}

func TestPageControlOnTheFindChainCounts(t *testing.T) {
	cases := map[string]string{
		"direct Limit":        `db.Limit(10).Find(&rows)`,
		"direct Scopes":       `db.Scopes(page.Scope).Find(&rows)`,
		"Limit in the middle": `db.Where("a = ?", 1).Limit(5).Order("id").Find(&rows)`,
		"Scopes after Where":  `db.Where("a = ?", 1).Scopes(page.Scope).Find(&rows)`,
	}
	for name, stmt := range cases {
		t.Run(name, func(t *testing.T) {
			hasFind, hasUnpagedFind, _ := scanBody(parseBody(t, stmt))
			if !hasFind {
				t.Fatalf("%s: expected a result-set Find", stmt)
			}
			if hasUnpagedFind {
				t.Fatalf("%s: page control on the Find chain should satisfy the gate", stmt)
			}
		})
	}
}

func TestFindWithoutPageControlIsReported(t *testing.T) {
	hasFind, hasUnpagedFind, _ := scanBody(parseBody(t, `db.Where("a = ?", 1).Find(&rows)`))
	if !hasFind || !hasUnpagedFind {
		t.Fatal("a Find with no page control must be reported")
	}
}

// Find(&x) taking a non-pointer first argument is not a result-set query, so
// it should not be reported either way.
func TestNonPointerFindIsIgnored(t *testing.T) {
	hasFind, _, _ := scanBody(parseBody(t, `db.Find(single)`))
	if hasFind {
		t.Fatal("Find with a non-pointer argument is not a list query")
	}
}

func TestBatchInIsDetected(t *testing.T) {
	_, _, hasBatchIN := scanBody(parseBody(t, `db.Where("id IN ?", ids).Find(&rows)`))
	if !hasBatchIN {
		t.Fatal("an IN(?) batch fetch should be recognised")
	}
}
