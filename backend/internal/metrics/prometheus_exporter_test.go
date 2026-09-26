package metrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestServeExposesRegisteredMetrics 锁定 D-06：指标注册在 registry 里，但没有
// 任何端点暴露它们，等于没有埋点。这个测试确认端点真的能返回已注册的指标。
func TestServeExposesRegisteredMetrics(t *testing.T) {
	// 高位端口，避免与常用服务冲突；测试结束即关闭。
	const addr = "127.0.0.1:19090"

	server := Serve(addr, zerolog.Nop())
	defer Shutdown(context.Background(), server, zerolog.Nop())

	var (
		response *http.Response
		err      error
	)
	// Listener 在 goroutine 里启动，给它一点时间绑定端口。
	for attempt := 0; attempt < 50; attempt++ {
		response, err = http.Get("http://" + addr + "/metrics")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("metrics endpoint never became reachable: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", response.StatusCode)
	}

	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatalf("read metrics body failed: %v", readErr)
	}
	rendered := string(body)

	// 端点本身可用：标准 registry 至少会带 Go runtime / process 指标。
	if !strings.Contains(rendered, "go_gc_duration_seconds") {
		t.Fatalf("expected Go runtime metrics on the endpoint, got %d bytes", len(rendered))
	}

	// 业务指标是 *Vec 类型：在写入过某个 label 组合之前，client_golang 不会把
	// 它们输出（没有样本就没有描述符）。所以这里先模拟一次中间件记录，
	// 验证"埋点 -> 暴露"这条链路，而不是只验证端口开着。
	//
	// registry 是进程级共享的：写入样本后必须清理，否则本包另一个用例
	// （TestPrometheusMiddleware 会对该 collector 做全量输出比对）会看到
	// 多出来的 label 组合而失败。清理放在 defer 里，断言失败也照样执行。
	defer HttpRequestsTotal.DeleteLabelValues("GET", "/metrics-exporter-smoke", "200")
	defer HttpRequestDuration.DeleteLabelValues("GET", "/metrics-exporter-smoke")

	HttpRequestsTotal.WithLabelValues("GET", "/metrics-exporter-smoke", "200").Inc()
	HttpRequestDuration.WithLabelValues("GET", "/metrics-exporter-smoke").Observe(0.01)

	response2, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("second scrape failed: %v", err)
	}
	defer response2.Body.Close()
	after, readErr := io.ReadAll(response2.Body)
	if readErr != nil {
		t.Fatalf("read second metrics body failed: %v", readErr)
	}

	for _, want := range []string{"http_requests_total", "http_request_duration_seconds"} {
		if !strings.Contains(string(after), want) {
			t.Fatalf("expected %q in the /metrics output after recording a sample", want)
		}
	}
}

// TestServeOtherPathsNotExposed 端点只提供 /metrics，不应把别的路径也打开。
func TestServeOtherPathsNotExposed(t *testing.T) {
	const addr = "127.0.0.1:19091"
	server := Serve(addr, zerolog.Nop())
	defer Shutdown(context.Background(), server, zerolog.Nop())

	var response *http.Response
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		response, err = http.Get("http://" + addr + "/api/v1/users")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("listener never became reachable: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on non-metrics paths, got %d", response.StatusCode)
	}
}

func TestShutdownIsNilSafe(t *testing.T) {
	// 配置关闭时 metricsServer 为 nil，退出路径不能 panic。
	Shutdown(context.Background(), nil, zerolog.Nop())
}
