package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 装配契约测试（P0-5）。
//
// 代码审查发现：Phase 0-2 交付的 internal/tenant、internal/alerting、
// internal/kms 三个包**整包零外部引用**——实现完整、单测全绿，却从未接进运行时。
// 根因是"新增包 + 单测"与"装配到 main.go"之间没有任何强制关联，
// 而 cmd/server（装配入口）本身零测试，CI 完全无法发现这类断点。
//
// 本文件用两类断言把"已接线"变成 CI 可验证的事实：
//   1. 关键包必须存在自身之外的引用（结构性断言，防死代码回归）；
//   2. main.go 必须出现关键装配调用（装配点断言）。

// scanImports 收集 root 下非测试 Go 文件的 import 路径 -> 引用文件列表。
func scanImports(t *testing.T, root string) map[string][]string {
	t.Helper()
	out := make(map[string][]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			// 生成代码/语法异常文件跳过，不影响契约判断。
			return nil
		}
		for _, imp := range file.Imports {
			pkg := strings.Trim(imp.Path.Value, `"`)
			out[pkg] = append(out[pkg], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk backend: %v", err)
	}
	return out
}

// TestCriticalPackagesAreWired 断言关键包不是死代码。
func TestCriticalPackagesAreWired(t *testing.T) {
	// 本测试位于 backend/cmd/server，向上两级即 backend 根。
	const backendRoot = ".." + string(filepath.Separator) + ".."

	imports := scanImports(t, backendRoot)

	// 包名 -> 中文说明（失败信息要能直接看懂影响面）。
	critical := map[string]string{
		"internal/tenant":   "租户隔离中间件",
		"internal/alerting": "告警分级路由",
		"internal/kms":      "KMS 主密钥管理",
	}

	const module = "github.com/allcallall/backend/"
	for suffix, desc := range critical {
		pkg := module + suffix
		files := imports[pkg]
		ownDir := filepath.Join(backendRoot, filepath.FromSlash(suffix))

		external := 0
		for _, f := range files {
			if strings.HasPrefix(f, ownDir+string(filepath.Separator)) {
				continue // 包内自引用不算接线
			}
			external++
		}
		if external == 0 {
			t.Errorf(
				"%s（%s）没有任何外部引用，属于死代码：实现与单测都在，但没有接进运行时。"+
					"请在 cmd/server 或 internal/runtime 完成装配。",
				desc, pkg,
			)
		}
	}
}

// TestMainWiresCriticalServices 断言 main.go 里的关键装配点仍然存在。
func TestMainWiresCriticalServices(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)

	wants := map[string]string{
		"appruntime.TenantResolverFromService":    "租户解析器装配",
		"appruntime.AlertingFromEnv":              "告警服务装配",
		"commerce.NewOrgBillingService":           "组织计费服务实例化",
		"commerce.NewInvoiceService":              "发票服务实例化",
		"commerce.NewUsageStatsService":           "用量统计服务实例化",
		"commerce.NewQuotaService":                "配额服务实例化",
		"appruntime.ApplyPrivacyPolicies(rootCtx": "隐私策略装配（须传 ctx）",
		"OrgBilling:":                             "组织计费 handler 注入路由依赖",
		"TenantResolver:":                         "租户中间件注入路由依赖",
	}

	for needle, desc := range wants {
		if !strings.Contains(body, needle) {
			t.Errorf("main.go 缺少装配点 %q（%s）——这是导致能力不生效的装配断点", needle, desc)
		}
	}
}
