package models

import (
	"time"
)

type UserStats struct {
	AcceptedCurrentWindow int `json:"accepted_current_window"`
	RejectedCumulative    int `json:"rejected_cumulative"`
}

type StatsResponse struct {
	UserStats
}

type RateLimitRequest struct {
	Payload map[string]interface{} `json:"payload"`
}

type Product struct {
	TenantID  string    `json:"-"`
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SKU       string    `json:"sku"`
	ImageURLs []string  `json:"image_urls"`
	VideoURLs []string  `json:"video_urls"`
	CreatedAt time.Time `json:"created_at"`
}

type ProductSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	ImageCount   int    `json:"image_count"`
	VideoCount   int    `json:"video_count"`
	ThumbnailURL string `json:"thumbnail_url"`
}

type ProductListResponse struct {
	Items  []ProductSummary `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Total  int              `json:"total"`
}

type ProductExportResponse struct {
	Items  []Product `json:"items"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
	Total  int       `json:"total"`
}

type AddMediaRequest struct {
	ImageURLs []string `json:"image_urls"`
	VideoURLs []string `json:"video_urls"`
}

type UpdateProductRequest struct {
	Name string `json:"name"`
	SKU  string `json:"sku"`
}
