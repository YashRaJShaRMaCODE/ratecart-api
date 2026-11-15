package handlers

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/middleware"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/models"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/services"
)

var errRequestTooLarge = errors.New("request body exceeds 1 MiB limit")

//go:embed openapi.json
var openAPISpec []byte

type Handler struct {
	rateService    *services.RateLimitService
	productService *services.ProductService
}

func NewHandler(rs *services.RateLimitService, ps *services.ProductService) *Handler {
	return &Handler{rateService: rs, productService: ps}
}

func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.productService.Ping(ctx); err != nil {
		jsonResponse(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) OpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func (h *Handler) HandleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	tenantID := middleware.TenantIDFromContext(r.Context())
	if tenantID == "" {
		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	var req models.RateLimitRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]string{"status": "accepted"})
}

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	tenantID := middleware.TenantIDFromContext(r.Context())
	if tenantID == "" {
		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	stats, err := h.rateService.GetStats(r.Context(), tenantID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "could not read request stats"})
		return
	}
	jsonResponse(w, http.StatusOK, stats)
}

func (h *Handler) HandleProducts(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantIDFromContext(r.Context())
	if tenantID == "" {
		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	switch r.Method {
	case http.MethodPost:
		var p models.Product
		if err := decodeJSON(w, r, &p); err != nil {
			writeDecodeError(w, err)
			return
		}
		created, err := h.productService.CreateProduct(r.Context(), tenantID, &p)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "conflict") {
				status = http.StatusConflict
			} else if errors.Is(err, services.ErrCatalogUnavailable) {
				status = http.StatusInternalServerError
				jsonResponse(w, status, map[string]string{"error": "could not save product"})
				return
			}
			jsonResponse(w, status, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusCreated, created)

	case http.MethodGet:
		limit, err := queryInt(r, "limit", 20)
		if err != nil || limit <= 0 {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		offset, err := queryInt(r, "offset", 0)
		if err != nil || offset < 0 {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "offset must be a non-negative integer"})
			return
		}
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		items, total, err := h.productService.ListProducts(r.Context(), tenantID, limit, offset)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "could not list products"})
			return
		}
		jsonResponse(w, http.StatusOK, models.ProductListResponse{
			Items:  items,
			Limit:  limit,
			Offset: offset,
			Total:  total,
		})
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (h *Handler) HandleProductExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	tenantID := middleware.TenantIDFromContext(r.Context())
	if tenantID == "" {
		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	limit, err := queryInt(r, "limit", 25)
	if err != nil || limit <= 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil || offset < 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "offset must be a non-negative integer"})
		return
	}
	if limit > 25 {
		limit = 25
	}
	items, total, err := h.productService.ListProductsDetailed(r.Context(), tenantID, limit, offset)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "could not export products"})
		return
	}
	jsonResponse(w, http.StatusOK, models.ProductExportResponse{Items: items, Limit: limit, Offset: offset, Total: total})
}

func (h *Handler) HandleProductByID(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantIDFromContext(r.Context())
	if tenantID == "" {
		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/products/")
	if id == "" {
		h.HandleProducts(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, ok, err := h.productService.GetByID(r.Context(), tenantID, id)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "could not read product"})
			return
		}
		if !ok {
			jsonResponse(w, http.StatusNotFound, map[string]string{"error": "product not found"})
			return
		}
		jsonResponse(w, http.StatusOK, p)

	case http.MethodPost:
		if !strings.HasSuffix(r.URL.Path, "/media") {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		id = strings.TrimSuffix(id, "/media")
		var req models.AddMediaRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}
		if err := h.productService.AddMedia(r.Context(), tenantID, id, req); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, services.ErrProductNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(err, services.ErrInvalidMedia) || strings.Contains(err.Error(), "at least one media array required") {
				status = http.StatusBadRequest
			} else {
				jsonResponse(w, status, map[string]string{"error": "could not update product media"})
				return
			}
			jsonResponse(w, status, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "media added"})
	case http.MethodPut:
		if strings.Contains(id, "/") {
			jsonResponse(w, http.StatusNotFound, map[string]string{"error": "product not found"})
			return
		}
		var req models.UpdateProductRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}
		if err := h.productService.UpdateProduct(r.Context(), tenantID, id, req); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, services.ErrProductNotFound) {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), "conflict") {
				status = http.StatusConflict
			} else if errors.Is(err, services.ErrCatalogUnavailable) {
				jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "could not update product"})
				return
			}
			jsonResponse(w, status, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "product updated"})
	case http.MethodDelete:
		if strings.Contains(id, "/") {
			jsonResponse(w, http.StatusNotFound, map[string]string{"error": "product not found"})
			return
		}
		if err := h.productService.DeleteProduct(r.Context(), tenantID, id); err != nil {
			if errors.Is(err, services.ErrProductNotFound) {
				jsonResponse(w, http.StatusNotFound, map[string]string{"error": "product not found"})
			} else {
				jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "could not delete product"})
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "GET, POST, PUT, DELETE")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if isRequestTooLarge(err) {
			return errRequestTooLarge
		}
		return errors.New("invalid JSON request body")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if isRequestTooLarge(err) {
			return errRequestTooLarge
		}
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func isRequestTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}

func writeDecodeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, errRequestTooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	jsonResponse(w, status, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func queryInt(r *http.Request, key string, fallback int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func jsonResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}
