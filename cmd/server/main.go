package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/adham/hotel-qr-ordering/internal/handler"
	"github.com/adham/hotel-qr-ordering/internal/middleware"
	"github.com/adham/hotel-qr-ordering/internal/repository"
	"github.com/adham/hotel-qr-ordering/internal/service"
)

func main() {
	// Load environment variables from .env file if present
	_ = godotenv.Load()

	log.Println("Starting Hotel QR Ordering Backend...")

	// 1. Load Configurations from Env with defaults
	port := getEnv("PORT", "8080")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/hotel_ordering?sslmode=disable")
	redisURL := getEnv("REDIS_URL", "localhost:6379")
	migrationsDir := getEnv("MIGRATIONS_DIR", "./db")

	// 2. Set up Root Context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. Initialize Postgres Repository & Run Migrations/Seeds
	dbRepo, err := repository.NewPostgresRepository(dbURL)
	if err != nil {
		log.Fatalf("Fatal: Database initialization failed: %v", err)
	}
	defer dbRepo.Close()

	if err := dbRepo.RunMigrations(migrationsDir); err != nil {
		log.Fatalf("Fatal: Database migrations failed: %v", err)
	}

	// 4. Initialize Redis Repository
	redisRepo, err := repository.NewRedisRepository(redisURL, "", 0)
	if err != nil {
		log.Fatalf("Fatal: Redis initialization failed: %v", err)
	}
	defer func() {
		if err := redisRepo.Close(); err != nil {
			log.Printf("Error closing Redis client: %v", err)
		}
	}()

	// 5. Initialize Storage Repository (MinIO / S3 compatible)
	storageRepo, err := repository.NewS3StorageRepository(ctx)
	if err != nil {
		log.Fatalf("Fatal: Storage repository initialization failed: %v", err)
	}

	// 6. Initialize & Start WebSocket Hub
	hub := handler.NewWSHub(redisRepo, dbRepo)
	hub.Start(ctx)

	// 7. Initialize Service Layer & Handler Layer
	srv := service.NewHotelService(dbRepo, redisRepo)
	h := handler.NewHTTPHandler(srv, hub, storageRepo)

	// 8. Setup Gin Server
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Static routes for local fallback and QR code folder downloads
	_ = os.MkdirAll("./public/uploads", 0755)
	_ = os.MkdirAll("./public/qrcodes", 0755)
	r.Static("/uploads", "./public/uploads")
	r.Static("/qrcodes", "./public/qrcodes")

	// CORS Middleware (Dynamic origin reflection with whitelist support)
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := false

		if origin != "" {
			// 1. Whitelist all localhosts for development
			if strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1") || strings.HasPrefix(origin, "https://localhost") {
				allowed = true
			}

			// 2. Whitelist private/LAN IPs (RFC1918) for mobile/WiFi testing
			if !allowed {
				if strings.HasPrefix(origin, "http://192.168.") || strings.HasPrefix(origin, "http://10.") || strings.HasPrefix(origin, "http://172.") {
					allowed = true
				}
			}

			// 3. Whitelist production domains from environmental configs
			if !allowed {
				prodWhitelist := os.Getenv("ORIGIN_WHITELIST") // Comma-separated list
				if prodWhitelist != "" {
					domains := strings.Split(prodWhitelist, ",")
					for _, d := range domains {
						if strings.TrimSpace(d) == origin {
							allowed = true
							break
						}
					}
				}
			}
		}

		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			// Fallback: Default to localhost dev UI or omit to block unauthorized origins
			c.Writer.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Redirect root & guest to Next.js Frontend Order Page (using default seed token)
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "http://localhost:3000/order?token=token_101_grand")
	})

	r.GET("/guest", func(c *gin.Context) {
		token := c.DefaultQuery("token", "token_101_grand")
		c.Redirect(http.StatusFound, "http://localhost:3000/order?token="+token)
	})

	// Public API Routes Group
	api := r.Group("/api/v1")
	{
		// Auth
		api.POST("/auth/signup", h.Signup)
		api.POST("/auth/login", h.Login)

		// Client
		api.POST("/client/session/negotiate", h.NegotiateSession)
		api.GET("/client/bootstrap", h.GetBootstrap)
		api.GET("/client/orders", h.GetClientOrders)
		api.POST("/orders", h.CreateOrder) // Public order placement
	}

	// Secured Admin Routes Group
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuthMiddleware())
	{
		admin.GET("/services", h.GetPropertyServices)
		admin.PATCH("/services/toggle", h.ToggleService)
		admin.POST("/catalog/upload", h.UploadCatalogImage)
		
		admin.GET("/rooms", h.GetRooms)
		admin.POST("/rooms", h.CreateRoom)
		admin.POST("/rooms/:id/checkout", h.CheckoutRoom)
		admin.POST("/rooms/:id/rotate", h.RotateRoomToken)
		admin.GET("/rooms/:id/qr", h.GetRoomQR)

		admin.GET("/catalog", h.GetCatalogItems)
		admin.POST("/catalog", h.CreateCatalogItem)
		admin.PUT("/catalog/:id", h.UpdateCatalogItem)
		admin.DELETE("/catalog/:id", h.DeleteCatalogItem)
		
		admin.GET("/orders", h.GetOrders)
		admin.PATCH("/orders/:id/status", h.UpdateOrderStatus)
		admin.DELETE("/orders/:id/items/:item_id", h.RemoveOrderItem)
	}

	// WebSocket upgrading endpoint
	r.GET("/ws/admin", hub.ServeWS)
	r.GET("/ws/client", hub.ServeWS)

	// 8. Start HTTP Server with Graceful Shutdown
	srvAddr := ":" + port
	server := &http.Server{
		Addr:    srvAddr,
		Handler: r,
	}

	go func() {
		log.Printf("Server listening on http://localhost:%s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Fatal: Listen and serve failed: %v", err)
		}
	}()

	// Wait for OS shutdown signal
	<-ctx.Done()
	log.Println("Shutdown signal received, shutting down server gracefully...")

	// Context for server graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Fatal: Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped successfully")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
