package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/middleware"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/services"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/store"
)

const (
	demoTenantKey  = "demo-tenant-key-012345678901234567890123"
	otherTenantKey = "other-tenant-key-01234567890123456789012"
)

type unhealthyCatalog struct {
	store.Catalog
	err error
}

func (s unhealthyCatalog) Ping(context.Context) error { return s.err }

func testHandler() *Handler {
	data := store.NewInMemoryStore()
	return NewHandler(services.NewRateLimitService(data), services.NewProductService(data))
}

func TestProductsRejectsNegativeOffset(t *testing.T) {
	if res := call(testHandler(), http.MethodGet, "/products?offset=-1", ""); res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestHealthCheckReportsStorageOutageWithoutLeakingDetails(t *testing.T) {
	if got := call(testHandler(), http.MethodGet, "/health", ""); got.Code != http.StatusOK {
		t.Fatalf("in-memory readiness status = %d, want %d", got.Code, http.StatusOK)
	}

	data := store.NewInMemoryStore()
	catalog := unhealthyCatalog{Catalog: data, err: errors.New("private database detail")}
	h := NewHandler(services.NewRateLimitService(data), services.NewProductService(catalog))
	res := call(h, http.MethodGet, "/health", "")

	if res.Code != http.StatusServiceUnavailable || res.Body.String() != `{"status":"unavailable"}`+"\n" {
		t.Fatalf("unexpected health response: %d %q", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "private database detail") {
		t.Fatalf("health response leaked storage details: %s", res.Body.String())
	}
}

func TestOpenAPISpecIsPublicCachedAndDescribesCurrentRoutes(t *testing.T) {
	h := testHandler()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	res := httptest.NewRecorder()
	h.OpenAPISpec(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected OpenAPI response: %d %#v", res.Code, res.Header())
	}
	if res.Header().Get("Cache-Control") != "public, max-age=3600" || res.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing OpenAPI response headers: %#v", res.Header())
	}
	var spec struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &spec); err != nil {
		t.Fatalf("invalid OpenAPI JSON: %v", err)
	}
	if spec.OpenAPI != "3.1.0" || len(spec.Paths) != 6 {
		t.Fatalf("unexpected OpenAPI version or paths: version=%q paths=%v", spec.OpenAPI, spec.Paths)
	}
	for _, path := range []string{"/health", "/request", "/stats", "/products", "/products/{id}", "/products/{id}/media"} {
		if _, ok := spec.Paths[path]; !ok {
			t.Errorf("OpenAPI spec missing %s", path)
		}
	}
	if _, ok := spec.Paths["/health"]["get"]; !ok {
		t.Error("OpenAPI spec must describe health GET")
	}
	if _, ok := spec.Paths["/products/{id}"]["put"]; !ok {
		t.Error("OpenAPI spec must describe product update PUT")
	}
	if _, ok := spec.Paths["/products/{id}"]["delete"]; !ok {
		t.Error("OpenAPI spec must describe product delete DELETE")
	}
}

func TestProductsRejectsUnsupportedMethod(t *testing.T) {
	res := call(testHandler(), http.MethodDelete, "/products", "")
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
	if res.Header().Get("Allow") != "GET, POST" || !strings.Contains(res.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("unexpected method error headers: %#v", res.Header())
	}
}

func TestRequestRejectsExtraAndUnknownJSON(t *testing.T) {
	for _, body := range []string{
		`{"payload":{}} {"payload":{}}`,
		`{"payload":{},"user_id":"caller-controlled"}`,
	} {
		res := call(testHandler(), http.MethodPost, "/request", body)
		if res.Code != http.StatusBadRequest {
			t.Errorf("status = %d for rejected request body, want %d", res.Code, http.StatusBadRequest)
		}
	}
}

func TestOversizedJSONBodyReturns413(t *testing.T) {
	body := `{"payload":{"data":"` + strings.Repeat("x", 1<<20) + `"}}`
	res := call(testHandler(), http.MethodPost, "/request", body)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(res.Body.String(), "1 MiB") {
		t.Fatalf("unexpected error body: %s", res.Body.String())
	}
}

func TestRequestRateLimitUsesAuthenticatedTenant(t *testing.T) {
	h := testHandler()
	for i := 0; i < 5; i++ {
		res := call(h, http.MethodPost, "/request", `{"payload":{}}`)
		if res.Code != http.StatusCreated {
			t.Fatalf("request %d status = %d, want %d", i+1, res.Code, http.StatusCreated)
		}
	}
	limited := call(h, http.MethodPost, "/request", `{"payload":{}}`)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth request status = %d, want %d", limited.Code, http.StatusTooManyRequests)
	}
	if limited.Header().Get("Retry-After") != "60" || limited.Header().Get("X-RateLimit-Limit") != "5" || limited.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("quota response missing retry guidance: %#v", limited.Header())
	}
	stats := call(h, http.MethodGet, "/stats", "")
	if stats.Code != http.StatusOK || !strings.Contains(stats.Body.String(), `"accepted_current_window":5`) || !strings.Contains(stats.Body.String(), `"rejected_cumulative":1`) {
		t.Fatalf("unexpected tenant stats: %d %s", stats.Code, stats.Body.String())
	}
	otherStats := callAs(h, otherTenantKey, http.MethodGet, "/stats", "")
	if otherStats.Code != http.StatusOK || !strings.Contains(otherStats.Body.String(), `"accepted_current_window":0`) || !strings.Contains(otherStats.Body.String(), `"rejected_cumulative":0`) {
		t.Fatalf("another tenant saw unexpected stats: %d %s", otherStats.Code, otherStats.Body.String())
	}
}

func TestProductCreateListAndAddMedia(t *testing.T) {
	h := testHandler()
	created := call(h, http.MethodPost, "/products", `{"name":"Desk lamp","sku":"LAMP-001"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", created.Code, http.StatusCreated, created.Body.String())
	}
	var product struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &product); err != nil || product.ID == "" {
		t.Fatalf("created product has no ID: %v", err)
	}
	if other := call(h, http.MethodPost, "/products", `{"name":"Desk chair","sku":"CHAIR-001"}`); other.Code != http.StatusCreated {
		t.Fatalf("second create status = %d, want %d: %s", other.Code, http.StatusCreated, other.Body.String())
	}

	listed := call(h, http.MethodGet, "/products?limit=1", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"sku":"LAMP-001"`) || !strings.Contains(listed.Body.String(), `"total":2`) {
		t.Fatalf("unexpected product list response: %d %s", listed.Code, listed.Body.String())
	}

	media := call(h, http.MethodPost, "/products/"+product.ID+"/media", `{"image_urls":["https://example.com/lamp.png"]}`)
	if media.Code != http.StatusOK {
		t.Fatalf("add media status = %d: %s", media.Code, media.Body.String())
	}
	got := call(h, http.MethodGet, "/products/"+product.ID, "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "https://example.com/lamp.png") {
		t.Fatalf("unexpected product detail response: %d %s", got.Code, got.Body.String())
	}
	if got.Header().Get("Cache-Control") != "no-store" || got.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing safe response headers: %#v", got.Header())
	}
}

func TestProductUpdateIsTenantScopedAndRejectsDuplicateSKU(t *testing.T) {
	h := testHandler()
	created := call(h, http.MethodPost, "/products", `{"name":"Desk lamp","sku":"LAMP-001"}`)
	var product struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &product); err != nil || product.ID == "" {
		t.Fatalf("created product has no ID: %v", err)
	}
	if second := call(h, http.MethodPost, "/products", `{"name":"Desk chair","sku":"CHAIR-001"}`); second.Code != http.StatusCreated {
		t.Fatalf("second create status = %d: %s", second.Code, second.Body.String())
	}

	updated := call(h, http.MethodPut, "/products/"+product.ID, `{"name":"Reading lamp","sku":"LAMP-002"}`)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), "product updated") {
		t.Fatalf("update status = %d: %s", updated.Code, updated.Body.String())
	}
	got := call(h, http.MethodGet, "/products/"+product.ID, "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"name":"Reading lamp"`) || !strings.Contains(got.Body.String(), `"sku":"LAMP-002"`) {
		t.Fatalf("updated product = %d %s", got.Code, got.Body.String())
	}
	if conflict := call(h, http.MethodPut, "/products/"+product.ID, `{"name":"Lamp","sku":"CHAIR-001"}`); conflict.Code != http.StatusConflict {
		t.Fatalf("duplicate SKU update status = %d, want %d: %s", conflict.Code, http.StatusConflict, conflict.Body.String())
	}
	if hidden := callAs(h, otherTenantKey, http.MethodPut, "/products/"+product.ID, `{"name":"Other","sku":"OTHER-1"}`); hidden.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant update status = %d, want %d: %s", hidden.Code, http.StatusNotFound, hidden.Body.String())
	}
}

func TestProductDeleteIsTenantScopedAndReleasesSKU(t *testing.T) {
	h := testHandler()
	created := call(h, http.MethodPost, "/products", `{"name":"Desk lamp","sku":"LAMP-DELETE"}`)
	var product struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &product); err != nil || product.ID == "" {
		t.Fatalf("created product has no ID: %v", err)
	}
	if got := callAs(h, otherTenantKey, http.MethodDelete, "/products/"+product.ID, ""); got.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant delete status = %d, want 404", got.Code)
	}
	if got := call(h, http.MethodGet, "/products/"+product.ID, ""); got.Code != http.StatusOK {
		t.Fatalf("cross-tenant delete affected owner product: %d", got.Code)
	}
	if got := call(h, http.MethodDelete, "/products/"+product.ID, ""); got.Code != http.StatusNoContent || got.Body.Len() != 0 {
		t.Fatalf("owner delete = %d %q, want empty 204", got.Code, got.Body.String())
	}
	if got := call(h, http.MethodGet, "/products/"+product.ID, ""); got.Code != http.StatusNotFound {
		t.Fatalf("deleted product lookup status = %d, want 404", got.Code)
	}
	if got := call(h, http.MethodPost, "/products", `{"name":"Replacement lamp","sku":"LAMP-DELETE"}`); got.Code != http.StatusCreated {
		t.Fatalf("reusing deleted SKU status = %d: %s", got.Code, got.Body.String())
	}
}

func TestCatalogRequestsShareTenantRateLimit(t *testing.T) {
	h := testHandler()
	for i := 0; i < 5; i++ {
		if got := call(h, http.MethodGet, "/products", "").Code; got != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, got, http.StatusOK)
		}
	}
	limited := call(h, http.MethodPost, "/products", `{"name":"Lamp","sku":"LAMP-1"}`)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth tenant API request status = %d, want 429", limited.Code)
	}
	if seconds, err := strconv.Atoi(limited.Header().Get("Retry-After")); err != nil || seconds < 1 || seconds > 60 {
		t.Fatalf("Retry-After = %q, want 1..60 seconds", limited.Header().Get("Retry-After"))
	}
	if got := callAs(h, otherTenantKey, http.MethodGet, "/products", "").Code; got != http.StatusOK {
		t.Fatalf("another tenant's quota was consumed: status = %d", got)
	}
}

func TestProductsAreIsolatedByAuthenticatedTenant(t *testing.T) {
	h := testHandler()
	created := call(h, http.MethodPost, "/products", `{"name":"Lamp","sku":"ITEM-1"}`)
	var product struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &product); err != nil || product.ID == "" {
		t.Fatalf("created product has no ID: %v", err)
	}
	if got := callAs(h, otherTenantKey, http.MethodGet, "/products/"+product.ID, ""); got.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant detail status = %d, want 404", got.Code)
	}
	if got := callAs(h, otherTenantKey, http.MethodGet, "/products", ""); !strings.Contains(got.Body.String(), `"total":0`) {
		t.Fatalf("other tenant list contains another tenant's product: %s", got.Body.String())
	}
	if got := callAs(h, otherTenantKey, http.MethodPost, "/products", `{"name":"Other lamp","sku":"ITEM-1"}`); got.Code != http.StatusCreated {
		t.Fatalf("same SKU in another tenant status = %d: %s", got.Code, got.Body.String())
	}
}

func call(h *Handler, method, path, body string) *httptest.ResponseRecorder {
	return callAs(h, demoTenantKey, method, path, body)
}

func callAs(h *Handler, apiKey, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res := httptest.NewRecorder()
	var handler http.HandlerFunc
	switch {
	case path == "/products":
		handler = h.HandleProducts
	case strings.HasPrefix(path, "/products/"):
		handler = h.HandleProductByID
	case strings.HasPrefix(path, "/request"):
		handler = h.HandleRequest
	case strings.HasPrefix(path, "/health"):
		handler = h.HealthCheck
	case strings.HasPrefix(path, "/stats"):
		handler = h.HandleStats
	default:
		handler = h.HandleProducts
	}
	auth, err := middleware.NewAPIKeyAuthenticator(`[{"tenant_id":"demo","key":"` + demoTenantKey + `"},{"tenant_id":"other","key":"` + otherTenantKey + `"}]`)
	if err != nil {
		panic(err)
	}
	protected := http.Handler(handler)
	if path != "/stats" && !strings.HasPrefix(path, "/health") {
		protected = middleware.RateLimit(h.rateService.ProcessRequest)(protected)
	}
	auth.Middleware(protected).ServeHTTP(res, req)
	return res
}
