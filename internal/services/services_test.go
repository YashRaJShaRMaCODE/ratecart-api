package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/models"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/store"
)

func TestCreateProductTrimsAndRejectsDuplicateSKU(t *testing.T) {
	service := NewProductService(store.NewInMemoryStore())
	first, err := service.CreateProduct(context.Background(), "demo", &models.Product{Name: "  Lamp ", SKU: " lamp-1 "})
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "Lamp" || first.SKU != "lamp-1" {
		t.Fatalf("unexpected normalized product: %#v", first)
	}
	if first.ImageURLs == nil || first.VideoURLs == nil {
		t.Fatalf("missing media should be normalized to empty arrays: %#v", first)
	}
	if _, err := service.CreateProduct(context.Background(), "demo", &models.Product{Name: "Another", SKU: "lamp-1"}); err == nil {
		t.Fatal("expected duplicate SKU to fail")
	}
	if _, err := service.CreateProduct(context.Background(), "other", &models.Product{Name: "Other lamp", SKU: "lamp-1"}); err != nil {
		t.Fatalf("same SKU in another tenant failed: %v", err)
	}
}

func TestUpdateProductNormalizesFieldsAndKeepsTenantScope(t *testing.T) {
	service := NewProductService(store.NewInMemoryStore())
	product, err := service.CreateProduct(context.Background(), "demo", &models.Product{Name: "Lamp", SKU: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateProduct(context.Background(), "demo", product.ID, models.UpdateProductRequest{Name: "  Desk lamp ", SKU: " new "}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := service.GetByID(context.Background(), "demo", product.ID)
	if err != nil || !ok || got.Name != "Desk lamp" || got.SKU != "new" {
		t.Fatalf("updated product = (%#v, %v, %v)", got, ok, err)
	}
	if err := service.UpdateProduct(context.Background(), "other", product.ID, models.UpdateProductRequest{Name: "Other", SKU: "other"}); err != ErrProductNotFound {
		t.Fatalf("cross-tenant update error = %v, want ErrProductNotFound", err)
	}
}

func TestConcurrentCreateAllowsOnlyOneSKU(t *testing.T) {
	service := NewProductService(store.NewInMemoryStore())
	var succeeded atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.CreateProduct(context.Background(), "demo", &models.Product{Name: "Lamp", SKU: "same"}); err == nil {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := succeeded.Load(); got != 1 {
		t.Fatalf("successful creates = %d, want 1", got)
	}
}

func TestValidateMediaRequiresAbsoluteHTTPURL(t *testing.T) {
	for _, url := range []string{"//example.com/image.png", "file:///tmp/image.png", "https:///missing-host"} {
		if err := validateMedia([]string{url}, nil); err == nil {
			t.Errorf("validateMedia(%q) succeeded, want error", url)
		}
	}
}
