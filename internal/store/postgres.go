package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/models"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresCatalog struct {
	db *sql.DB
}

func OpenPostgresCatalog(ctx context.Context, dsn string) (*PostgresCatalog, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open catalog database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect catalog database: %w", err)
	}
	return &PostgresCatalog{db: db}, nil
}

func (s *PostgresCatalog) Close() error { return s.db.Close() }

func (s *PostgresCatalog) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *PostgresCatalog) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS ratecart_products (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			sku TEXT NOT NULL,
			image_urls JSONB NOT NULL DEFAULT '[]'::jsonb,
			video_urls JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMPTZ NOT NULL,
			CONSTRAINT ratecart_products_tenant_sku_unique UNIQUE (tenant_id, sku)
		);
	`); err != nil {
		return fmt.Errorf("migrate catalog database: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS ratecart_products_tenant_created_idx ON ratecart_products (tenant_id, created_at, id)`); err != nil {
		return fmt.Errorf("index catalog database: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ratecart_requests (
		tenant_id TEXT NOT NULL,
		requested_at TIMESTAMPTZ NOT NULL
	)`); err != nil {
		return fmt.Errorf("migrate request quota database: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS ratecart_requests_tenant_time_idx ON ratecart_requests (tenant_id, requested_at)`); err != nil {
		return fmt.Errorf("index request quota database: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ratecart_rate_counters (
		tenant_id TEXT PRIMARY KEY,
		rejected_cumulative BIGINT NOT NULL DEFAULT 0 CHECK (rejected_cumulative >= 0)
	)`); err != nil {
		return fmt.Errorf("migrate request counters: %w", err)
	}
	return nil
}

func (s *PostgresCatalog) ProcessRequest(ctx context.Context, tenantID string) (bool, time.Duration, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, fmt.Errorf("begin quota transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenantID); err != nil {
		return false, 0, fmt.Errorf("lock tenant quota: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ratecart_requests WHERE tenant_id = $1 AND requested_at <= clock_timestamp() - interval '1 minute'`, tenantID); err != nil {
		return false, 0, fmt.Errorf("prune tenant quota: %w", err)
	}
	var accepted int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ratecart_requests WHERE tenant_id = $1`, tenantID).Scan(&accepted); err != nil {
		return false, 0, fmt.Errorf("count tenant quota: %w", err)
	}
	if accepted >= 5 {
		var retryAfter time.Duration
		if err := tx.QueryRowContext(ctx, `SELECT GREATEST(ceil(EXTRACT(EPOCH FROM (MIN(requested_at) + interval '1 minute' - clock_timestamp()))), 1)::bigint
			FROM ratecart_requests WHERE tenant_id = $1`, tenantID).Scan(&retryAfter); err != nil {
			return false, 0, fmt.Errorf("read quota reset time: %w", err)
		}
		retryAfter *= time.Second
		if _, err := tx.ExecContext(ctx, `INSERT INTO ratecart_rate_counters (tenant_id, rejected_cumulative) VALUES ($1, 1)
			ON CONFLICT (tenant_id) DO UPDATE SET rejected_cumulative = ratecart_rate_counters.rejected_cumulative + 1`, tenantID); err != nil {
			return false, 0, fmt.Errorf("count rejected request: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return false, 0, fmt.Errorf("commit rejected request: %w", err)
		}
		return false, retryAfter, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ratecart_requests (tenant_id, requested_at) VALUES ($1, clock_timestamp())`, tenantID); err != nil {
		return false, 0, fmt.Errorf("record accepted request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, 0, fmt.Errorf("commit accepted request: %w", err)
	}
	return true, 0, nil
}

func (s *PostgresCatalog) ReadStats(ctx context.Context, tenantID string) (models.UserStats, error) {
	var stats models.UserStats
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM ratecart_requests WHERE tenant_id = $1
		AND requested_at > clock_timestamp() - interval '1 minute'`, tenantID).Scan(&stats.AcceptedCurrentWindow); err != nil {
		return models.UserStats{}, fmt.Errorf("read accepted request count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT rejected_cumulative FROM ratecart_rate_counters WHERE tenant_id = $1), 0)`, tenantID).Scan(&stats.RejectedCumulative); err != nil {
		return models.UserStats{}, fmt.Errorf("read rejected request count: %w", err)
	}
	return stats, nil
}

func (s *PostgresCatalog) CreateProduct(ctx context.Context, p *models.Product) error {
	images, err := marshalURLs(p.ImageURLs)
	if err != nil {
		return fmt.Errorf("encode product images: %w", err)
	}
	videos, err := marshalURLs(p.VideoURLs)
	if err != nil {
		return fmt.Errorf("encode product videos: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO ratecart_products (id, tenant_id, name, sku, image_urls, video_urls, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, p.ID, p.TenantID, p.Name, p.SKU, images, videos, p.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ratecart_products_tenant_sku_unique" {
			return ErrDuplicateSKU
		}
		return fmt.Errorf("save product: %w", err)
	}
	return nil
}

func (s *PostgresCatalog) GetProductByID(ctx context.Context, tenantID, id string) (*models.Product, bool, error) {
	var p models.Product
	var images, videos []byte
	err := s.db.QueryRowContext(ctx, `SELECT id, tenant_id, name, sku, image_urls, video_urls, created_at
		FROM ratecart_products WHERE tenant_id = $1 AND id = $2`, tenantID, id).
		Scan(&p.ID, &p.TenantID, &p.Name, &p.SKU, &images, &videos, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read product: %w", err)
	}
	if err := json.Unmarshal(images, &p.ImageURLs); err != nil {
		return nil, false, fmt.Errorf("decode product images: %w", err)
	}
	if err := json.Unmarshal(videos, &p.VideoURLs); err != nil {
		return nil, false, fmt.Errorf("decode product videos: %w", err)
	}
	return &p, true, nil
}

func (s *PostgresCatalog) ListProducts(ctx context.Context, tenantID string, limit, offset int) ([]models.ProductSummary, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM ratecart_products WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, sku, jsonb_array_length(image_urls), jsonb_array_length(video_urls),
		COALESCE(image_urls->>0, '') FROM ratecart_products WHERE tenant_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	items := make([]models.ProductSummary, 0)
	for rows.Next() {
		var item models.ProductSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.SKU, &item.ImageCount, &item.VideoCount, &item.ThumbnailURL); err != nil {
			return nil, 0, fmt.Errorf("scan product summary: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate products: %w", err)
	}
	return items, total, nil
}

func (s *PostgresCatalog) ListProductsDetailed(ctx context.Context, tenantID string, limit, offset int) ([]models.Product, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM ratecart_products WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tenant products for export: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, tenant_id, name, sku, image_urls, video_urls, created_at
		FROM ratecart_products WHERE tenant_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenant products for export: %w", err)
	}
	defer rows.Close()
	items := make([]models.Product, 0, min(limit, total))
	for rows.Next() {
		var p models.Product
		var images, videos []byte
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.SKU, &images, &videos, &p.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan tenant product for export: %w", err)
		}
		if err := json.Unmarshal(images, &p.ImageURLs); err != nil {
			return nil, 0, fmt.Errorf("decode product images for export: %w", err)
		}
		if err := json.Unmarshal(videos, &p.VideoURLs); err != nil {
			return nil, 0, fmt.Errorf("decode product videos for export: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read tenant products for export: %w", err)
	}
	return items, total, nil
}

func (s *PostgresCatalog) AddProductMedia(ctx context.Context, tenantID, id string, images, videos []string) (bool, error) {
	imageJSON, err := marshalURLs(images)
	if err != nil {
		return false, fmt.Errorf("encode image URLs: %w", err)
	}
	videoJSON, err := marshalURLs(videos)
	if err != nil {
		return false, fmt.Errorf("encode video URLs: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE ratecart_products SET image_urls = image_urls || $1::jsonb, video_urls = video_urls || $2::jsonb
		WHERE tenant_id = $3 AND id = $4`, imageJSON, videoJSON, tenantID, id)
	if err != nil {
		return false, fmt.Errorf("append product media: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check product media update: %w", err)
	}
	return count == 1, nil
}

func (s *PostgresCatalog) UpdateProduct(ctx context.Context, tenantID, id, name, sku string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE ratecart_products SET name = $1, sku = $2
		WHERE tenant_id = $3 AND id = $4`, name, sku, tenantID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ratecart_products_tenant_sku_unique" {
			return false, ErrDuplicateSKU
		}
		return false, fmt.Errorf("update product: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check product update: %w", err)
	}
	return count == 1, nil
}

func (s *PostgresCatalog) DeleteProduct(ctx context.Context, tenantID, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM ratecart_products WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return false, fmt.Errorf("delete product: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check product deletion: %w", err)
	}
	return count == 1, nil
}

func marshalURLs(urls []string) ([]byte, error) {
	if urls == nil {
		urls = []string{}
	}
	return json.Marshal(urls)
}
