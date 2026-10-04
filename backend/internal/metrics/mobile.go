package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics for the paths a mobile user actually depends on.
//
// Until now the only signal about realtime was chat_realtime_delivery_failures
// and the only signal about push was a log line. A deployment where nobody can
// receive a call, or where signalling is refusing every connection, produced no
// metric and no alert - the dashboards were green because nothing was counting
// the thing that was broken.
//
// These are deliberately few. A rule that can never fire is worse than no rule
// (the dashboard looks guarded while nobody is watching), so each one here is
// something an on-call engineer can act on at 3am.

var (
	// SignalingConnections is the number of live realtime connections in this
	// process. Watched as a pair with the connection-accept rate: a sudden drop
	// to zero during working hours means clients cannot connect at all, which is
	// the single most damaging failure this product can have.
	SignalingConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "signaling_connections",
		Help: "Current number of connected realtime clients in this process.",
	})

	// SignalingConnectionsTotal counts connection lifecycle events by outcome.
	SignalingConnectionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "signaling_connections_total",
		Help: "Realtime connection attempts by outcome (accepted, rejected, closed).",
	}, []string{"outcome"})

	// SignalingDisconnectsTotal counts why connections ended. A spike in
	// transport errors points at the network or the edge; a spike in
	// application-level closes points at this service.
	SignalingDisconnectsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "signaling_disconnects_total",
		Help: "Realtime connections closed, by reason.",
	}, []string{"reason"})

	// PushNotificationsTotal is the outcome of every call notification attempt.
	// "skipped" is the one that used to be invisible: push was disabled, the
	// send returned nil, and the count of "sent" devices climbed anyway.
	PushNotificationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "push_notifications_total",
		Help: "Call notification delivery attempts by outcome (sent, failed, skipped, no_token).",
	}, []string{"outcome"})

	// PushNotificationDuration measures how long a fan-out takes, including the
	// provider round trip. A rise here is visible to users as a ring that starts
	// late.
	PushNotificationDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "push_notification_duration_seconds",
		Help:    "Time to complete a call-notification fan-out to one recipient's devices.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	})

	// CallSetupTotal counts call establishment by terminal outcome. A user who
	// cannot complete a call is the failure this product exists to avoid, and it
	// had no metric at all.
	//
	// There is deliberately no call_setup_duration histogram: SignalMessage
	// carries no timestamp, so invite-to-connected cannot be measured without
	// adding one to the wire format. An unimplemented duration would be worse
	// than none - it reads as coverage in a dashboard while showing nothing.
	CallSetupTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "call_setup_total",
		Help: "Call setup attempts by outcome (completed, rejected, cancelled).",
	}, []string{"outcome"})

	// RateLimitRejectedTotal counts requests refused by the global limiter.
	// A rise means either abuse or a limit tuned too low for mobile NAT
	// patterns, and those need different responses.
	RateLimitRejectedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "rate_limit_rejected_total",
		Help: "Requests rejected by the global rate limiter.",
	})
)

func init() {
	prometheus.MustRegister(
		SignalingConnections,
		SignalingConnectionsTotal,
		SignalingDisconnectsTotal,
		PushNotificationsTotal,
		PushNotificationDuration,
		CallSetupTotal,
		RateLimitRejectedTotal,
	)
}
