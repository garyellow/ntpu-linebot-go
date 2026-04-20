package genai

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimitKind distinguishes transient burst throttling from longer-lived quota exhaustion.
type RateLimitKind string

const (
	RateLimitNone      RateLimitKind = ""
	RateLimitBurst     RateLimitKind = "burst"
	RateLimitExhausted RateLimitKind = "exhausted"
)

const (
	defaultBurstCooldown     = 8 * time.Second
	minBurstCooldown         = 2 * time.Second
	maxBurstCooldown         = 30 * time.Second
	defaultExhaustedCooldown = 15 * time.Minute
	minExhaustedCooldown     = 5 * time.Minute
	maxExhaustedCooldown     = 1 * time.Hour
	longRetryAfterThreshold  = 1 * time.Minute
)

type modelCooldown struct {
	Until  time.Time
	Kind   RateLimitKind
	Reason string
}

func (c modelCooldown) Remaining(now time.Time) time.Duration {
	return c.Until.Sub(now)
}

type modelCooldownStore struct {
	mu      sync.RWMutex
	entries map[string]modelCooldown
}

func newModelCooldownStore() *modelCooldownStore {
	return &modelCooldownStore{entries: make(map[string]modelCooldown)}
}

var globalModelCooldownStore = newModelCooldownStore()

func cooldownKey(provider Provider, model string) string {
	return fmt.Sprintf("%s:%s", provider, model)
}

func (s *modelCooldownStore) Get(provider Provider, model string) (modelCooldown, bool) {
	if s == nil {
		return modelCooldown{}, false
	}
	key := cooldownKey(provider, model)
	now := time.Now()

	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return modelCooldown{}, false
	}
	if !entry.Until.After(now) {
		s.mu.Lock()
		delete(s.entries, key)
		s.mu.Unlock()
		return modelCooldown{}, false
	}
	return entry, true
}

func (s *modelCooldownStore) Set(provider Provider, model string, kind RateLimitKind, d time.Duration, reason string) modelCooldown {
	if s == nil {
		return modelCooldown{}
	}
	entry := modelCooldown{
		Until:  time.Now().Add(d),
		Kind:   kind,
		Reason: reason,
	}
	s.mu.Lock()
	s.entries[cooldownKey(provider, model)] = entry
	s.mu.Unlock()
	return entry
}

func classifyRateLimitKind(err error) RateLimitKind {
	if err == nil {
		return RateLimitNone
	}

	errStr := strings.ToLower(err.Error())
	has429Signal := containsAny(errStr,
		"429",
		"too many requests",
		"rate limit",
		"resource_exhausted",
		"requests per second",
		"requests per minute",
		"tokens per minute",
		"rps",
		"rpm",
		"tpm",
	)

	var llmErr *LLMError
	if errors.As(err, &llmErr) && llmErr.StatusCode == http.StatusTooManyRequests {
		has429Signal = true
		if retryAfter := ParseRetryAfter(llmErr.Headers); retryAfter > longRetryAfterThreshold {
			return RateLimitExhausted
		}
	}

	if !has429Signal {
		return RateLimitNone
	}

	if containsAny(errStr,
		"quota exceeded",
		"daily quota",
		"daily limit",
		"monthly limit",
		"billing",
		"credit",
		"spend limit",
		"requests per day",
		"tokens per day",
		"rpd",
		"tpd",
	) {
		return RateLimitExhausted
	}

	return RateLimitBurst
}

func inferStatusCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	errStr := strings.ToLower(err.Error())
	switch {
	case containsAny(errStr, "429", "too many requests", "rate limit", "resource_exhausted"):
		return http.StatusTooManyRequests
	case containsAny(errStr, "401", "unauthorized", "invalid api key", "invalid_api_key"):
		return http.StatusUnauthorized
	case containsAny(errStr, "403", "forbidden", "permission denied"):
		return http.StatusForbidden
	case containsAny(errStr, "404", "not found"):
		return http.StatusNotFound
	case containsAny(errStr, "408", "timeout", "deadline"):
		return http.StatusRequestTimeout
	case containsAny(errStr, "409", "conflict"):
		return http.StatusConflict
	case containsAny(errStr, "422", "unprocessable"):
		return http.StatusUnprocessableEntity
	case containsAny(errStr, "400", "bad request", "invalid request", "malformed"):
		return http.StatusBadRequest
	case containsAny(errStr, "500", "internal server error"):
		return http.StatusInternalServerError
	case containsAny(errStr, "502", "bad gateway"):
		return http.StatusBadGateway
	case containsAny(errStr, "503", "service unavailable", "overloaded", "capacity"):
		return http.StatusServiceUnavailable
	case containsAny(errStr, "504", "gateway timeout"):
		return http.StatusGatewayTimeout
	default:
		return 0
	}
}

func cooldownDurationFor(kind RateLimitKind, err error) time.Duration {
	base := time.Duration(0)
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		base = ParseRetryAfter(llmErr.Headers)
	}

	switch kind {
	case RateLimitExhausted:
		if base < minExhaustedCooldown {
			base = defaultExhaustedCooldown
		}
		if base > maxExhaustedCooldown {
			base = maxExhaustedCooldown
		}
		return base
	case RateLimitBurst:
		if base < minBurstCooldown {
			base = defaultBurstCooldown
		}
		if base > maxBurstCooldown {
			base = maxBurstCooldown
		}
		return base
	default:
		return 0
	}
}

func applyCooldown(store *modelCooldownStore, provider Provider, model string, err error) (modelCooldown, bool) {
	kind := classifyRateLimitKind(err)
	if kind == RateLimitNone {
		return modelCooldown{}, false
	}
	d := cooldownDurationFor(kind, err)
	if d <= 0 {
		return modelCooldown{}, false
	}
	return store.Set(provider, model, kind, d, err.Error()), true
}
