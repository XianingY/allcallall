package metrics

// Recorder provides a unified interface for recording system metrics.
type Recorder interface {
	Inc(name string)
	Add(name string, delta int64)
	Set(name string, value int64)
}
