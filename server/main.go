package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/san-kum/motorcycle-cv/server/cache"
	"github.com/san-kum/motorcycle-cv/server/config"
	"github.com/san-kum/motorcycle-cv/server/handlers"
	"github.com/san-kum/motorcycle-cv/server/middleware"
	"github.com/san-kum/motorcycle-cv/server/ml"
	"github.com/san-kum/motorcycle-cv/server/processor"
	"go.uber.org/zap"
)

type Server struct {
	router         *gin.Engine
	logger         *zap.Logger
	frameProcessor *processor.FrameProcessor
	mlClient       *ml.Client
	cache          cache.Cache
	rateLimiter    *middleware.RateLimiter
	config         *config.Config
}

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize logger
	var logger *zap.Logger
	var err error

	if cfg.Logging.Format == "json" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}

	if err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}
	defer logger.Sync()

	// Validate configuration
	if err := cfg.ValidateConfig(logger); err != nil {
		logger.Fatal("Configuration validation failed", zap.Error(err))
	}

	// Set Gin mode
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	server, err := NewServer(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create server", zap.Error(err))
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      server.router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		logger.Info("Starting server",
			zap.String("addr", addr),
			zap.String("environment", cfg.Server.Environment))

		var err error
		if cfg.Security.EnableHTTPS {
			err = srv.ListenAndServeTLS(cfg.Security.CertFile, cfg.Security.KeyFile)
		} else {
			err = srv.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown frame processor
	if err := server.frameProcessor.Shutdown(); err != nil {
		logger.Error("Failed to shutdown frame processor", zap.Error(err))
	}

	// Shutdown rate limiter
	if server.rateLimiter != nil {
		server.rateLimiter.Shutdown()
	}

	// Shutdown cache
	if server.cache != nil {
		if err := server.cache.Close(); err != nil {
			logger.Error("Failed to close cache", zap.Error(err))
		}
	}

	// Shutdown HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

func NewServer(cfg *config.Config, logger *zap.Logger) (*Server, error) {
	// Initialize cache
	var cacheInstance cache.Cache
	var err error

	// Try Redis first, fallback to memory cache
	if cfg.Redis.Host != "" {
		cacheInstance, err = cache.NewRedisCache(
			cfg.Redis.Host,
			cfg.Redis.Port,
			cfg.Redis.Password,
			cfg.Redis.DB,
			5*time.Minute, // Default TTL
			logger,
		)
		if err != nil {
			logger.Warn("Failed to connect to Redis, using memory cache", zap.Error(err))
			cacheInstance = cache.NewMemoryCache(1000, 5*time.Minute, logger)
		}
	} else {
		cacheInstance = cache.NewMemoryCache(1000, 5*time.Minute, logger)
	}

	// Initialize ML client
	mlClient, err := ml.NewClient(cfg.ML.BaseURL, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create ML client: %w", err)
	}

	// Initialize frame processor
	frameProcessor := processor.NewFrameProcessor(mlClient, cacheInstance, logger)

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter(
		cfg.Security.RateLimitRPS,
		cfg.Security.RateLimitBurst,
		logger,
	)

	// Initialize authentication middleware
	authMiddleware := middleware.NewAuthMiddleware(cfg.Security.JWTSecretKey, logger)

	// Setup router
	router := gin.New()

	// Add middleware
	router.Use(middleware.RequestLogger(logger))
	router.Use(gin.Recovery())
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.CORS(cfg.Security.AllowedOrigins))
	router.Use(middleware.RequestSizeLimit(cfg.Security.MaxRequestSize))
	router.Use(middleware.InputValidation())
	router.Use(middleware.TimeoutHandler(cfg.Security.RequestTimeout))

	// Initialize handlers
	wsHandler := handlers.NewWebSocketHandler(frameProcessor, logger)
	streamHandler := handlers.NewStreamHandler(frameProcessor, logger)

	// Setup routes
	setupRoutes(router, wsHandler, streamHandler, authMiddleware, rateLimiter)

	return &Server{
		router:         router,
		logger:         logger,
		frameProcessor: frameProcessor,
		mlClient:       mlClient,
		cache:          cacheInstance,
		rateLimiter:    rateLimiter,
		config:         cfg,
	}, nil
}

func setupRoutes(router *gin.Engine, wsHandler *handlers.WebSocketHandler, streamHandler *handlers.StreamHandler, auth *middleware.AuthMiddleware, rateLimiter *middleware.RateLimiter) {
	// Health check (no auth required)
	router.GET("/health", middleware.HealthCheck())

	// WebSocket endpoint (rate limited)
	router.GET("/ws", rateLimiter.RateLimit(), wsHandler.HandleWebSocket)

	// API routes
	api := router.Group("/api/v1")
	{
		// Public endpoints
		api.GET("/health", middleware.HealthCheck())

		// Protected endpoints
		protected := api.Group("/")
		protected.Use(rateLimiter.RateLimit())
		{
			// Frame analysis (rate limited)
			protected.POST("/analyze-frame", streamHandler.ProcessFrame)

			// Statistics (rate limited)
			protected.GET("/stats", streamHandler.GetStats)

			// Video upload (rate limited)
			protected.POST("/upload-video", streamHandler.UploadVideo)
			protected.GET("/video-job/:job_id", streamHandler.GetVideoJobStatus)
		}

		// Admin endpoints (require authentication)
		admin := api.Group("/admin")
		admin.Use(auth.RequireAuth())
		admin.Use(auth.RequireRole("admin"))
		{
			admin.GET("/stats", streamHandler.GetStats)
			admin.GET("/cache-stats", func(c *gin.Context) {
				// This would need to be implemented in the handler
				c.JSON(http.StatusOK, gin.H{"message": "Cache stats endpoint"})
			})
		}
	}

	// Static files
	router.Static("/static", "./client")
	router.StaticFile("/", "./client/index.html")
}
