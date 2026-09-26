package metrics

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

var (
	HttpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HttpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(HttpRequestsTotal)
	prometheus.MustRegister(HttpRequestDuration)
}

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		method := c.Request.Method

		HttpRequestsTotal.WithLabelValues(method, path, status).Inc()
		HttpRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}

// Serve 在独立地址暴露标准 Prometheus 抓取端点，返回已启动的 server 供调用方
// 做优雅退出。
//
// 与 /api/v1/metrics 是两套东西：后者是自研 CounterStore 的文本渲染，供已有
// Grafana 面板使用；这里是标准 registry，包含上面两个指标以及 Go runtime 和
// process 指标。放在独立端口而不是挂进 API 路由，是为了能用 NetworkPolicy 把
// 访问限制到只有 Prometheus，而不必给业务中间件链加例外。
//
// Serve starts the standard Prometheus scrape listener on its own address.
func Serve(addr string, logger zerolog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		logger.Info().Str("addr", addr).Msg("prometheus metrics listener starting")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("prometheus metrics listener failed")
		}
	}()

	return server
}

// Shutdown 优雅停止 metrics 端点。
func Shutdown(ctx context.Context, server *http.Server, logger zerolog.Logger) {
	if server == nil {
		return
	}
	if err := server.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("prometheus metrics listener shutdown error")
		return
	}
	logger.Info().Msg("prometheus metrics listener stopped")
}
