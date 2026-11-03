package broadcast

import (
	"context"
	"time"
)

// Config contains configuration for broadcast processing
type Config struct {
	// Concurrency settings
	MaxParallelism int           `json:"max_parallelism"`
	MaxProcessTime time.Duration `json:"max_process_time"`

	// Batch processing
	FetchBatchSize   int `json:"fetch_batch_size"`
	ProcessBatchSize int `json:"process_batch_size"`

	// Logging and metrics
	ProgressLogInterval time.Duration `json:"progress_log_interval"`

	// Circuit breaker settings
	EnableCircuitBreaker    bool          `json:"enable_circuit_breaker"`
	CircuitBreakerThreshold int           `json:"circuit_breaker_threshold"`
	CircuitBreakerCooldown  time.Duration `json:"circuit_breaker_cooldown"`

	// Rate limiting
	DefaultRateLimit int `json:"default_rate_limit"` // Emails per minute (fallback when broadcast doesn't specify rate limit)

	// Retry settings
	MaxRetries    int           `json:"max_retries"`
	RetryInterval time.Duration `json:"retry_interval"`

	// OnBroadcastSent, when set, runs after a broadcast completes with at least one recipient.
	// Optional: nil means nothing is notified, which is what every test gets.
	OnBroadcastSent BroadcastSentHook `json:"-"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		MaxParallelism:          10,
		MaxProcessTime:          50 * time.Second,
		FetchBatchSize:          50,
		ProcessBatchSize:        25,
		ProgressLogInterval:     5 * time.Second,
		EnableCircuitBreaker:    true,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  1 * time.Minute,
		DefaultRateLimit:        25, // 25 per minute
		MaxRetries:              3,
		RetryInterval:           30 * time.Second,
	}
}

// TestConfig returns a configuration optimized for fast unit tests
// with rate limiting disabled for speed
func TestConfig() *Config {
	return &Config{
		MaxParallelism:          10,
		MaxProcessTime:          50 * time.Second,
		FetchBatchSize:          50,
		ProcessBatchSize:        25,
		ProgressLogInterval:     5 * time.Second,
		EnableCircuitBreaker:    true,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  1 * time.Minute,
		DefaultRateLimit:        6000, // 6000 per minute = 100/sec (effectively no rate limiting for tests)
		MaxRetries:              3,
		RetryInterval:           30 * time.Second,
	}
}

// BroadcastSentHook is called once a broadcast has been marked processed and at least one
// recipient was enqueued. It receives the template and the exact version the send used, because
// a template edited during a long send must not be archived at a version nobody received.
//
// It runs after the status is saved, so it cannot fail the send; an implementation that returns
// an error is expected to log it. The archive is wired here rather than imported, keeping the
// broadcast package free of a dependency on it.
type BroadcastSentHook func(ctx context.Context, workspaceID, broadcastID, templateID string, templateVersion int64)
