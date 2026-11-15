package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/models"
	"github.com/google/uuid"
)

func TestPostgresCatalogPersistsTenantProducts(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("RATECART_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set RATECART_TEST_DATABASE_URL to run Postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	catalog, err := OpenPostgresCatalog(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		catalog.Close()
		t.Fatal(err)
	}
	tenantID := "test-" + uuid.NewString()
	defer func() {
		_, _ = catalog.db.ExecContext(context.Background(), `DELETE FROM ratecart_products WHERE tenant_id = $1`, tenantID)
		_, _ = catalog.db.ExecContext(context.Background(), `DELETE FROM ratecart_requests WHERE tenant_id = $1`, tenantID)
		_, _ = catalog.db.ExecContext(context.Background(), `DELETE FROM ratecart_rate_counters WHERE tenant_id = $1`, tenantID)
		_ = catalog.Close()
	}()
	product := &models.Product{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      "Test lamp",
		SKU:       "TEST-1",
		ImageURLs: []string{"https://example.test/lamp.png"},
		VideoURLs: []string{},
		CreatedAt: time.Now().UTC(),
	}
	if err := catalog.CreateProduct(ctx, product); err != nil {
		t.Fatal(err)
	}
	if updated, err := catalog.UpdateProduct(ctx, tenantID, product.ID, "Updated test lamp", "TEST-2"); err != nil || !updated {
		t.Fatalf("update product = (%v, %v), want (true, nil)", updated, err)
	}
	if err := catalog.CreateProduct(ctx, &models.Product{ID: uuid.NewString(), TenantID: tenantID, SKU: "TEST-2"}); err != ErrDuplicateSKU {
		t.Fatalf("duplicate SKU error = %v, want ErrDuplicateSKU", err)
	}
	for i := 0; i < 6; i++ {
		allowed, retryAfter, err := catalog.ProcessRequest(ctx, tenantID)
		if err != nil || allowed != (i < 5) {
			t.Fatalf("quota request %d = (%v, %v), want allowed=%v", i+1, allowed, err, i < 5)
		}
		if i == 5 && (retryAfter <= 0 || retryAfter > time.Minute) {
			t.Fatalf("rejected quota request retry delay = %s, want (0, 1m]", retryAfter)
		}
	}
	if ok, err := catalog.AddProductMedia(ctx, tenantID, product.ID, nil, []string{"https://example.test/demo.mp4"}); err != nil || !ok {
		t.Fatalf("add media = (%v, %v), want (true, nil)", ok, err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatal(err)
	}
	catalog, err = OpenPostgresCatalog(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := catalog.ReadStats(ctx, tenantID)
	if err != nil || stats.AcceptedCurrentWindow != 5 || stats.RejectedCumulative != 1 {
		t.Fatalf("reopened quota stats = (%#v, %v)", stats, err)
	}
	got, ok, err := catalog.GetProductByID(ctx, tenantID, product.ID)
	if err != nil || !ok || got.Name != "Updated test lamp" || got.SKU != "TEST-2" || len(got.VideoURLs) != 1 || got.VideoURLs[0] != "https://example.test/demo.mp4" {
		t.Fatalf("reopened catalog product = (%#v, %v, %v)", got, ok, err)
	}
	items, total, err := catalog.ListProducts(ctx, tenantID, 10, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].ThumbnailURL != product.ImageURLs[0] {
		t.Fatalf("list = (%#v, %d, %v)", items, total, err)
	}
}
