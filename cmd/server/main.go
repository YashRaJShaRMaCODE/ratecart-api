package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/handlers"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/middleware"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/services"
	"github.com/YashRaJShaRMaCODE/ratecart-api/internal/store"
)

func main() {
	apiKeys, err := middleware.NewAPIKeyAuthenticator(os.Getenv("RATECART_API_KEYS"))
	if err != nil {
		log.Fatal(err)
	}

	// Initialize Store
	memStore := store.NewInMemoryStore()
	var catalog store.Catalog = memStore
	var limiter store.RateLimiter = memStore
	var closeCatalog func() error
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		postgresCatalog, err := store.OpenPostgresCatalog(ctx, dsn)
		if err == nil {
			err = postgresCatalog.Migrate(ctx)
		}
		cancel()
		if err != nil {
			log.Fatalf("initialize product catalog: %v", err)
		}
		catalog = postgresCatalog
		limiter = postgresCatalog
		closeCatalog = postgresCatalog.Close
	} else if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") {
		log.Fatal("DATABASE_URL is required in production; refusing to start with volatile product storage")
	}

	// Initialize Services
	rateLimitService := services.NewRateLimitService(limiter)
	productService := services.NewProductService(catalog)

	// Initialize Handlers
	h := handlers.NewHandler(rateLimitService, productService)

	// Setup Router
	mux := http.NewServeMux()
	protected := func(next http.HandlerFunc, rateLimited bool) http.Handler {
		handler := http.Handler(next)
		if rateLimited {
			handler = middleware.RateLimit(rateLimitService.ProcessRequest)(handler)
		}
		return middleware.Logging(apiKeys.Middleware(handler))
	}

	mux.HandleFunc("/health", h.HealthCheck)
	mux.HandleFunc("/openapi.json", h.OpenAPISpec)
	mux.Handle("/request", protected(h.HandleRequest, true))
	mux.Handle("/stats", protected(h.HandleStats, false))
	mux.Handle("/products", protected(h.HandleProducts, true))
	mux.Handle("/products/", protected(h.HandleProductByID, true))

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful Shutdown
	go func() {
		log.Printf("Server starting on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
	if closeCatalog != nil {
		if err := closeCatalog(); err != nil {
			log.Printf("close catalog database: %v", err)
		}
	}
}
