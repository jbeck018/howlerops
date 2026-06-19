package database

import (
	"sync/atomic"
	"time"
)

// connectionStats tracks request/error counters for non-pooled connections
// (Elasticsearch, MongoDB) in a goroutine-safe way. Queries on the same
// connection can run concurrently, so the counters use atomics rather than
// plain ints guarded by the connection's RWMutex (which protects the client).
type connectionStats struct {
	requestCount  atomic.Int64
	errorCount    atomic.Int64
	lastRequestAt atomic.Int64 // unix nanoseconds; 0 = never
}

// recordRequest counts a request and records when it started.
func (s *connectionStats) recordRequest(at time.Time) {
	s.requestCount.Add(1)
	s.lastRequestAt.Store(at.UnixNano())
}

// recordError counts a failed request.
func (s *connectionStats) recordError() {
	s.errorCount.Add(1)
}
