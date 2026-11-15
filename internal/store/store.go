package store

import (
	"sync"
	"time"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/models"
)

type InMemoryStore struct {
	mu           sync.RWMutex
	userWindows  map[string][]time.Time
	userRejects  map[string]int
	products     map[string]*models.Product
	productOrder []string
	skuMap       map[string]string
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		userWindows:  make(map[string][]time.Time),
		userRejects:  make(map[string]int),
		products:     make(map[string]*models.Product),
		productOrder: make([]string, 0),
		skuMap:       make(map[string]string),
	}
}

// Rate Limiting Logic
func (s *InMemoryStore) AllowRequest(userID string) bool {
	allowed, _ := s.allowRequest(userID)
	return allowed
}

func (s *InMemoryStore) allowRequest(userID string) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	window := now.Add(-time.Minute)

	// Clean old entries
	var active []time.Time
	for _, t := range s.userWindows[userID] {
		if t.After(window) {
			active = append(active, t)
		}
	}

	if len(active) >= 5 {
		s.userRejects[userID]++
		s.userWindows[userID] = active
		return false, time.Until(active[0].Add(time.Minute))
	}

	active = append(active, now)
	s.userWindows[userID] = active
	return true, 0
}

func (s *InMemoryStore) GetStats(tenantID string) models.UserStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	window := now.Add(-time.Minute)
	count := 0
	for _, requestTime := range s.userWindows[tenantID] {
		if requestTime.After(window) {
			count++
		}
	}
	return models.UserStats{AcceptedCurrentWindow: count, RejectedCumulative: s.userRejects[tenantID]}
}

// Product Logic
func (s *InMemoryStore) AddProduct(p *models.Product) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	skuKey := p.TenantID + ":" + p.SKU
	if _, exists := s.skuMap[skuKey]; exists {
		return false
	}

	copy := *p
	copy.ImageURLs = append([]string(nil), p.ImageURLs...)
	copy.VideoURLs = append([]string(nil), p.VideoURLs...)
	s.products[p.ID] = &copy
	s.productOrder = append(s.productOrder, p.ID)
	s.skuMap[skuKey] = p.ID
	return true
}

func (s *InMemoryStore) getProductByID(tenantID, id string) (*models.Product, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.products[id]
	if !ok || p.TenantID != tenantID {
		return nil, false
	}
	copy := *p
	copy.ImageURLs = append([]string(nil), p.ImageURLs...)
	copy.VideoURLs = append([]string(nil), p.VideoURLs...)
	return &copy, true
}

func (s *InMemoryStore) GetProductBySKU(tenantID, sku string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.skuMap[tenantID+":"+sku]
	return id, ok
}

func (s *InMemoryStore) listProducts(tenantID string, limit, offset int) ([]models.ProductSummary, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]*models.Product, 0, len(s.productOrder))
	for _, id := range s.productOrder {
		if product := s.products[id]; product.TenantID == tenantID {
			products = append(products, product)
		}
	}
	total := len(products)
	if limit <= 0 || offset < 0 || offset >= total {
		return []models.ProductSummary{}, total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	res := make([]models.ProductSummary, 0, end-offset)
	for i := offset; i < end; i++ {
		p := products[i]

		thumb := ""
		if len(p.ImageURLs) > 0 {
			thumb = p.ImageURLs[0]
		}

		res = append(res, models.ProductSummary{
			ID:           p.ID,
			Name:         p.Name,
			SKU:          p.SKU,
			ImageCount:   len(p.ImageURLs),
			VideoCount:   len(p.VideoURLs),
			ThumbnailURL: thumb,
		})
	}
	return res, total
}

func (s *InMemoryStore) UpdateProductMedia(tenantID, id string, imgs, vids []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.products[id]
	if !ok || p.TenantID != tenantID {
		return false
	}

	p.ImageURLs = append(p.ImageURLs, imgs...)
	p.VideoURLs = append(p.VideoURLs, vids...)
	return true
}
