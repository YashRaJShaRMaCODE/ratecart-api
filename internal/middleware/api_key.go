package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type APIKey struct {
	TenantID string `json:"tenant_id"`
	Key      string `json:"key"`
}

type credential struct {
	tenantID string
	digest   [sha256.Size]byte
}

type APIKeyAuthenticator struct {
	credentials []credential
}

func NewAPIKeyAuthenticator(raw string) (*APIKeyAuthenticator, error) {
	var configured []APIKey
	if err := json.Unmarshal([]byte(raw), &configured); err != nil || len(configured) == 0 {
		return nil, errors.New("RATECART_API_KEYS must be a non-empty JSON array")
	}

	seen := make(map[[sha256.Size]byte]struct{}, len(configured))
	credentials := make([]credential, 0, len(configured))
	for _, item := range configured {
		if !validTenantID(item.TenantID) || len(item.Key) < 32 || strings.TrimSpace(item.Key) != item.Key {
			return nil, errors.New("each API key needs a valid tenant_id and a 32-character key")
		}
		digest := sha256.Sum256([]byte(item.Key))
		if _, exists := seen[digest]; exists {
			return nil, errors.New("API keys must be unique")
		}
		seen[digest] = struct{}{}
		credentials = append(credentials, credential{tenantID: item.TenantID, digest: digest})
	}
	return &APIKeyAuthenticator{credentials: credentials}, nil
}

func (a *APIKeyAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeUnauthorized(w)
			return
		}

		candidate := strings.TrimPrefix(authorization, "Bearer ")
		digest := sha256.Sum256([]byte(candidate))
		tenantID := ""
		for _, item := range a.credentials {
			if subtle.ConstantTimeCompare(digest[:], item.digest[:]) == 1 {
				tenantID = item.tenantID
			}
		}
		if tenantID == "" {
			writeUnauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), tenantIDContextKey{}, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RateLimit(allow func(context.Context, string) (bool, time.Duration, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := TenantIDFromContext(r.Context())
			if tenantID == "" {
				writeUnauthorized(w)
				return
			}
			allowed, retryAfter, err := allow(r.Context(), tenantID)
			if err != nil {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"rate limit service unavailable"}`))
				return
			}
			if !allowed {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("X-RateLimit-Limit", "5")
				w.Header().Set("X-RateLimit-Remaining", "0")
				seconds := int64((retryAfter + time.Second - 1) / time.Second)
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func validTenantID(tenantID string) bool {
	if len(tenantID) == 0 || len(tenantID) > 64 {
		return false
	}
	for _, char := range tenantID {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') &&
			!(char >= '0' && char <= '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"invalid API key"}`))
}
