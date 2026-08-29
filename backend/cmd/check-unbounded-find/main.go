// Command check-unbounded-find 是一个轻量级静态检查，用于在 CI 中防止
// "面向用户的列表端点裸写 .Find(&...) 而不加 .Limit(...)" 的回归。
//
// 它会扫描 internal/ 下所有非测试 Go 文件，找出函数名以
// List/Search/Query/GetAll/FetchAll/FindAll 开头、函数体内存在 .Find(&...)
// 调用但既无 .Limit(...) 也无 "IN ?" 批量取数（按入参 ID 列表取数，规模由
// 调用方控制，属安全模式）的函数，并报告其位置。
//
// 使用方式：
//
//	go run ./cmd/check-unbounded-find            # 严格模式，发现即非零退出
//	go run ./cmd/check-unbounded-find -advisory  # 仅打印，不阻断 CI
//
// 存量违规通过 allowlist 豁免，待逐步治理后从 allowlist 移除；新增的未分页
// 列表端点（不在 allowlist 中）会被严格模式拦截。
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// listFuncRe 匹配"列表类"函数名。
var listFuncRe = regexp.MustCompile(`^(List|Search|Query|GetAll|FetchAll|FindAll)`)

// allowlist 豁免存量未分页列表端点（已扫描确认的全部 21 处）。
// 逐项治理后从名单移除；新增的未分页列表端点（不在名单中）会被严格模式拦截。
// 注：ListRecordings 已示范完成分页（经 pagination.Scope 应用 Limit），
// 由扫描器识别 .Scopes(...) 视为已分页，故不在此豁免名单中。
var allowlist = map[string]bool{
	"ListGroups":               true,
	"ListReadReceipts":         true,
	"ListConversations":        true,
	"ListDeals":                true,
	"ListDealActivities":       true,
	"ListContacts":             true,
	"ListContactsWithProfiles": true,
	"ListSourcesInGroup":       true,
	"ListVersionsBySource":     true,
	"ListChunksByVersion":      true,
	"ListTools":                true,
	"ListInstallations":        true,
	"ListSkills":               true,
	"ListByOwner":              true,
	"ListPushDevices":          true,
	"ListPipelines":            true,
	"ListOrganizationMembers":  true,
	"ListOrganizationInvites":  true,
	"ListTeamMembers":          true,
	"ListUserBlocks":           true,
}

type finding struct {
	file string
	line int
	fn   string
}

func main() {
	advisory := flag.Bool("advisory", false, "only report, do not fail CI")
	root := flag.String("root", "internal", "directory to scan")
	flag.Parse()

	var findings []finding
	fset := token.NewFileSet()
	err := filepath.Walk(*root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return checkFile(fset, path, &findings)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
		os.Exit(2)
	}

	for _, f := range findings {
		if allowlist[f.fn] {
			continue
		}
		fmt.Fprintf(os.Stderr, "%s:%d: list function %q issues unbounded .Find(&...) (missing .Limit and not an IN(?) batch)\n", f.file, f.line, f.fn)
	}

	if len(findings) == 0 {
		fmt.Println("check-unbounded-find: ok (no unbounded list Find)")
		return
	}
	// 统计未被豁免的数量。
	blocked := 0
	for _, f := range findings {
		if !allowlist[f.fn] {
			blocked++
		}
	}
	if blocked == 0 {
		fmt.Printf("check-unbounded-find: ok (%d exempted by allowlist)\n", len(findings))
		return
	}
	if *advisory {
		fmt.Printf("check-unbounded-find: advisory mode, %d finding(s) reported\n", blocked)
		return
	}
	fmt.Fprintf(os.Stderr, "check-unbounded-find: %d unbounded list Find(s) must be paginated\n", blocked)
	os.Exit(1)
}

func checkFile(fset *token.FileSet, path string, out *[]finding) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !listFuncRe.MatchString(fn.Name.Name) {
			continue
		}
		if fn.Body == nil {
			continue
		}
		hasFind, hasPageControl, hasBatchIN := scanBody(fn.Body)
		if hasFind && !hasPageControl && !hasBatchIN {
			*out = append(*out, finding{file: path, line: fset.Position(fn.Pos()).Line, fn: fn.Name.Name})
		}
	}
	return nil
}

// scanBody 返回：是否存在 .Find(&...)、是否已施加分页控制（.Limit( 或
// .Scopes(，后者通常经 pagination.Page.Scope 应用 Limit/Offset）、是否存在
// "IN ?" 批量取数。
func scanBody(body *ast.BlockStmt) (hasFind, hasPageControl, hasBatchIN bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "Find":
			// 仅当首个实参为取址表达式（&slice）时视为结果集查询。
			if len(call.Args) > 0 {
				if _, isAddr := call.Args[0].(*ast.UnaryExpr); isAddr {
					hasFind = true
				}
			}
		case "Limit", "Scopes":
			// .Scopes(pagination.Page.Scope) 会在作用域内施加 Limit/Offset，
			// 视为已分页，避免误报经 helper 分页的列表端点。
			hasPageControl = true
		case "Where":
			if containsBatchIN(call) {
				hasBatchIN = true
			}
		}
		return true
	})
	return
}

// containsBatchIN 检测 WHERE 条件中是否含 "IN ?" 形式的批量取数。
func containsBatchIN(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if strings.Contains(lit.Value, "IN ?") {
				return true
			}
		}
	}
	return false
}
