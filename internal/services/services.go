package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/models"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/store"
	"github.com/google/uuid"
)

var ErrProductNotFound = errors.New("not found")
var ErrInvalidMedia = errors.New("invalid media")
var ErrCatalogUnavailable = errors.New("catalog storage unavailable")

type ProductService struct {
	store store.Catalog
}

func NewProductService(s store.Catalog) *ProductService {
	return &ProductService{store: s}
}

func (s *ProductService) Ping(ctx context.Context) error { return s.store.Ping(ctx) }

func (s *ProductService) CreateProduct(ctx context.Context, tenantID string, p *models.Product) (*models.Product, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.SKU = strings.TrimSpace(p.SKU)
	p.TenantID = tenantID
	if p.ImageURLs == nil {
		p.ImageURLs = []string{}
	}
	if p.VideoURLs == nil {
		p.VideoURLs = []string{}
	}
	if p.Name == "" || p.SKU == "" {
		return nil, errors.New("name and sku are required")
	}
	if len(p.Name) > 120 || len(p.SKU) > 64 {
		return nil, errors.New("name must be at most 120 characters and sku at most 64")
	}

	if err := validateMedia(p.ImageURLs, p.VideoURLs); err != nil {
		return nil, err
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("generate product ID: %w", err)
	}
	p.ID = id.String()
	p.CreatedAt = time.Now()

	if err := s.store.CreateProduct(ctx, p); err != nil {
		if errors.Is(err, store.ErrDuplicateSKU) {
			return nil, fmt.Errorf("conflict: sku %s already exists", p.SKU)
		}
		return nil, fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}
	return p, nil
}

func (s *ProductService) AddMedia(ctx context.Context, tenantID, id string, req models.AddMediaRequest) error {
	if len(req.ImageURLs) == 0 && len(req.VideoURLs) == 0 {
		return fmt.Errorf("%w: at least one media array required", ErrInvalidMedia)
	}

	if err := validateMedia(req.ImageURLs, req.VideoURLs); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidMedia, err)
	}

	if ok, err := s.store.AddProductMedia(ctx, tenantID, id, req.ImageURLs, req.VideoURLs); err != nil {
		return fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	} else if !ok {
		return ErrProductNotFound
	}
	return nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, tenantID, id string, req models.UpdateProductRequest) error {
	name := strings.TrimSpace(req.Name)
	sku := strings.TrimSpace(req.SKU)
	if name == "" || sku == "" {
		return errors.New("name and sku are required")
	}
	if len(name) > 120 || len(sku) > 64 {
		return errors.New("name must be at most 120 characters and sku at most 64")
	}
	updated, err := s.store.UpdateProduct(ctx, tenantID, id, name, sku)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateSKU) {
			return fmt.Errorf("conflict: sku %s already exists", sku)
		}
		return fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}
	if !updated {
		return ErrProductNotFound
	}
	return nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, tenantID, id string) error {
	deleted, err := s.store.DeleteProduct(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}
	if !deleted {
		return ErrProductNotFound
	}
	return nil
}

func validateMedia(imgs, vids []string) error {
	if len(imgs) > 20 || len(vids) > 20 {
		return errors.New("max 20 URLs per array")
	}
	for _, list := range [][]string{imgs, vids} {
		for _, u := range list {
			if len(u) > 2048 {
				return errors.New("URL max length 2048 chars")
			}
			parsed, err := url.Parse(u)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return fmt.Errorf("invalid URL: %s", u)
			}
		}
	}
	return nil
}

type RateLimitService struct {
	store store.RateLimiter
}

func NewRateLimitService(s store.RateLimiter) *RateLimitService {
	return &RateLimitService{store: s}
}

func (s *RateLimitService) ProcessRequest(ctx context.Context, tenantID string) (bool, time.Duration, error) {
	return s.store.ProcessRequest(ctx, tenantID)
}

func (s *RateLimitService) GetStats(ctx context.Context, tenantID string) (models.StatsResponse, error) {
	stats, err := s.store.ReadStats(ctx, tenantID)
	return models.StatsResponse{UserStats: stats}, err
}

func (s *ProductService) ListProducts(ctx context.Context, tenantID string, limit, offset int) ([]models.ProductSummary, int, error) {
	return s.store.ListProducts(ctx, tenantID, limit, offset)
}

func (s *ProductService) GetByID(ctx context.Context, tenantID, id string) (*models.Product, bool, error) {
	return s.store.GetProductByID(ctx, tenantID, id)
}
