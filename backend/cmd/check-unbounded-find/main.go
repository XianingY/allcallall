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

// allowlist 豁免存量未分页列表端点。
//
// 治理进度：经审计确认的全部 21 处面向用户的裸 .Find(&...) 列表端点已统一
// 收敛为 .Limit(pagination.MaxLimit)（封顶 500，防止全表扫描 / OOM）；其中
// 3 个高基数列表端点（ListConversations / ListDeals / ListOrganizationMembers）
// 进一步接入了真实的 offset 分页信封（total/limit/offset/has_more），并由
// web/mobile 客户端以 useInfiniteQuery / 追加 offset 的方式消费。
//
// 因此本 allowlist 当前为空：任何新增的、未在白名单内且未施加分页控制的列表
// 函数都会被严格模式拦截，防止回归。
//
// 注：ListRecordings 经 pagination.Scope 应用 Limit，由扫描器识别 .Scopes(...)
// 视为已分页，故不在此豁免名单中。
var allowlist = map[string]bool{}

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
		hasFind, hasUnpagedFind, hasBatchIN := scanBody(fn.Body)
		if hasFind && hasUnpagedFind && !hasBatchIN {
			*out = append(*out, finding{file: path, line: fset.Position(fn.Pos()).Line, fn: fn.Name.Name})
		}
	}
	return nil
}

// scanBody 返回：是否存在 .Find(&...)、是否存在**未分页**的 .Find、是否存在
// "IN ?" 批量取数。
//
// 分页控制按每个 Find 自己的调用链判断，而不是"函数体里出现过 .Limit("。
// 后者会让一个函数内的多个查询互相掩护：只要其中一个分页了，同函数里另一个
// 无分页的 Find 就被一并豁免。列表函数恰好常在一个函数里查多张表，所以这个
// 假阴性会真实放过未分页的列表端点。
func scanBody(body *ast.BlockStmt) (hasFind, hasUnpagedFind, hasBatchIN bool) {
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
					if !chainHasPageControl(sel.X) {
						hasUnpagedFind = true
					}
				}
			}
		case "Where":
			if containsBatchIN(call) {
				hasBatchIN = true
			}
		}
		return true
	})
	return
}

// chainHasPageControl 沿 db.Limit(n).Where(...).Find(&x) 这类接收者链向上找
// .Limit( 或 .Scopes(。.Scopes(pagination.Page.Scope) 会在作用域内施加
// Limit/Offset，同样视为已分页。
//
// 通过中间变量分页（q := db.Model(&x); q = q.Limit(10); q.Find(&y)）无法在
// 语法层判断，会被报为未分页。这是有意的保守方向：误报需要人工确认，漏报
// 才是真正的风险。
func chainHasPageControl(expr ast.Expr) bool {
	for {
		switch node := expr.(type) {
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return false
			}
			if sel.Sel.Name == "Limit" || sel.Sel.Name == "Scopes" {
				return true
			}
			expr = sel.X
		case *ast.SelectorExpr:
			expr = node.X
		case *ast.ParenExpr:
			expr = node.X
		default:
			return false
		}
	}
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
