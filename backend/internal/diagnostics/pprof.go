// Package diagnostics exposes Go runtime profiling endpoints on a dedicated
// internal listener.
//
// The API server previously had no pprof surface: Prometheus counters show
// *that* latency or allocation pressure exists, but not *where*. This package
// mounts net/http/pprof on its own address so an operator can sample a live
// process with `go tool pprof` during an incident.
//
// Security model (mirrors internal/metrics):
//   - Separate port (default :6060), never routed through the public API
//     engine or its middleware chain.
//   - The Helm chart does not publish the port through a Service or Ingress;
//     in-cluster access requires an explicit NetworkPolicy allow, which is
//     disabled by default.
package diagnostics

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/rs/zerolog"
)

// Serve 在独立地址暴露 Go runtime 诊断端点（/debug/pprof/*），返回已启动的
// server 供调用方做优雅退出。
//
// 注意：import net/http/pprof 本身会把 handler 注册到 http.DefaultServeMux
// （标准库 init 行为，无法阻止）。本监听器使用私有 mux，且代码库中不允许
// 出现 `http.ListenAndServe(addr, nil)`——那会顺带把 DefaultServeMux 上的
// 诊断端点暴露出去。生产代码当前没有任何组件使用 DefaultServeMux。
func Serve(addr string, logger zerolog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		// profile/trace 是长请求：采样默认 30s，留足余量避免被写超时掐断。
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		logger.Info().Str("addr", addr).Msg("pprof diagnostics listener starting")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("pprof diagnostics listener failed")
		}
	}()

	return server
}

// Shutdown 优雅停止 pprof 诊断端点。
func Shutdown(ctx context.Context, server *http.Server, logger zerolog.Logger) {
	if server == nil {
		return
	}
	if err := server.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("pprof diagnostics listener shutdown error")
		return
	}
	logger.Info().Msg("pprof diagnostics listener stopped")
}
