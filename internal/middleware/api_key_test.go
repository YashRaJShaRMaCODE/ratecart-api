package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	firstKey  = "first-tenant-api-key-012345678901234567"
	secondKey = "second-tenant-api-key-0123456789012345"
)

func TestAPIKeyMiddlewareRequiresValidBearerKey(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator(`[{"tenant_id":"shop-a","key":"` + firstKey + `"}]`)
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(TenantIDFromContext(r.Context())))
	})

	for _, authorization := range []string{"", "Basic " + firstKey, "Bearer wrong"} {
		req := httptest.NewRequest(http.MethodGet, "/products", nil)
		req.Header.Set("Authorization", authorization)
		res := httptest.NewRecorder()
		auth.Middleware(next).ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized || !strings.Contains(res.Header().Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("authorization %q: status=%d headers=%v", authorization, res.Code, res.Header())
		}
		if strings.Contains(res.Body.String(), firstKey) {
			t.Fatal("error response leaked an API key")
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	req.Header.Set("Authorization", "Bearer "+firstKey)
	res := httptest.NewRecorder()
	auth.Middleware(next).ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Body.String() != "shop-a" {
		t.Fatalf("valid key did not resolve its tenant: status=%d body=%q", res.Code, res.Body.String())
	}
}

func TestAPIKeyConfigValidationAndRotation(t *testing.T) {
	for _, raw := range []string{"", "{}", `[{"tenant_id":"bad tenant","key":"` + firstKey + `"}]`, `[{"tenant_id":"shop-a","key":"short"}]`} {
		if _, err := NewAPIKeyAuthenticator(raw); err == nil {
			t.Errorf("config %q accepted, want validation error", raw)
		}
	}

	raw := `[{"tenant_id":"shop-a","key":"` + firstKey + `"},{"tenant_id":"shop-a","key":"` + secondKey + `"}]`
	if _, err := NewAPIKeyAuthenticator(raw); err != nil {
		t.Fatalf("two rotation keys for one tenant should work: %v", err)
	}
	duplicate := `[{"tenant_id":"shop-a","key":"` + firstKey + `"},{"tenant_id":"shop-b","key":"` + firstKey + `"}]`
	if _, err := NewAPIKeyAuthenticator(duplicate); err == nil {
		t.Fatal("duplicate API key assigned to different tenants was accepted")
	}
}
