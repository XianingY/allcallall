package server

import (
	"sync/atomic"
)

// Graceful shutdown coordination.
//
// A rolling update used to hard-cut every realtime connection: SIGTERM went
// straight to http.Server.Shutdown, which closes listeners and waits for
// in-flight requests, but a WebSocket is a hijacked connection - Shutdown
// neither waits for it nor closes it. Clients were left holding a dead socket
// until their own reconnect logic noticed, and a 10s deadline expired long
// before the 30s grace period did, so the remaining 20s were spent with HTTP
// already dead.
//
// The signal from Kubernetes is the same for a Pod that is going away and one
// that is merely not ready yet, so the only way to stop receiving traffic early
// is to fail the readiness probe. /health deliberately stays healthy:
// a Pod shutting down on purpose should not be restarted by its liveness probe.
var draining atomic.Bool

// BeginDrain marks this instance as shutting down. Readiness starts failing so
// the endpoint slice drops it, while the process keeps serving what it already
// has until Shutdown is called.
func BeginDrain() {
	draining.Store(true)
}

// IsDraining reports whether shutdown has begun.
func IsDraining() bool {
	return draining.Load()
}
