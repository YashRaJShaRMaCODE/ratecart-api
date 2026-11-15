package store

import (
	"context"
	"errors"
	"time"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/models"
)

var ErrDuplicateSKU = errors.New("SKU already exists for tenant")

type Catalog interface {
	Ping(context.Context) error
	CreateProduct(context.Context, *models.Product) error
	UpdateProduct(context.Context, string, string, string, string) (bool, error)
	GetProductByID(context.Context, string, string) (*models.Product, bool, error)
	ListProducts(context.Context, string, int, int) ([]models.ProductSummary, int, error)
	AddProductMedia(context.Context, string, string, []string, []string) (bool, error)
}

type RateLimiter interface {
	ProcessRequest(context.Context, string) (bool, time.Duration, error)
	ReadStats(context.Context, string) (models.UserStats, error)
}

func (s *InMemoryStore) Ping(context.Context) error { return nil }

func (s *InMemoryStore) CreateProduct(_ context.Context, p *models.Product) error {
	if !s.AddProduct(p) {
		return ErrDuplicateSKU
	}
	return nil
}

func (s *InMemoryStore) UpdateProduct(_ context.Context, tenantID, id, name, sku string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.products[id]
	if !ok || p.TenantID != tenantID {
		return false, nil
	}
	newSKUKey := tenantID + ":" + sku
	if existingID, exists := s.skuMap[newSKUKey]; exists && existingID != id {
		return false, ErrDuplicateSKU
	}
	delete(s.skuMap, tenantID+":"+p.SKU)
	p.Name = name
	p.SKU = sku
	s.skuMap[newSKUKey] = id
	return true, nil
}

func (s *InMemoryStore) GetProductByID(_ context.Context, tenantID, id string) (*models.Product, bool, error) {
	p, ok := s.getProductByID(tenantID, id)
	return p, ok, nil
}

func (s *InMemoryStore) ListProducts(_ context.Context, tenantID string, limit, offset int) ([]models.ProductSummary, int, error) {
	items, total := s.listProducts(tenantID, limit, offset)
	return items, total, nil
}

func (s *InMemoryStore) AddProductMedia(_ context.Context, tenantID, id string, images, videos []string) (bool, error) {
	return s.UpdateProductMedia(tenantID, id, images, videos), nil
}

func (s *InMemoryStore) ProcessRequest(_ context.Context, tenantID string) (bool, time.Duration, error) {
	allowed, retryAfter := s.allowRequest(tenantID)
	return allowed, retryAfter, nil
}

func (s *InMemoryStore) ReadStats(_ context.Context, tenantID string) (models.UserStats, error) {
	return s.GetStats(tenantID), nil
}
