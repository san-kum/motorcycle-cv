package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/san-kum/motorcycle-cv/server/handlers"
	"github.com/san-kum/motorcycle-cv/server/ml"
	"github.com/san-kum/motorcycle-cv/server/processor"
	"go.uber.org/zap"
)

type Server struct {
	router         *gin.Engine
	logger         *zap.Logger
	frameProcessor *processor.FrameProcessor
	mlClient       *ml.Client
}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}
	defer logger.Sync()

	server, err := NewServer(logger)
	if err != nil {
		logger.Fatal("Failed to create server", zap.Error(err))
	}

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      server.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Starting server on port 8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

func NewServer(logger *zap.Logger) (*Server, error) {
	// Initialize the ML service client for communication with PyTorch service
	mlClient, err := ml.NewClient("http://localhost:5000", logger)
	if err != nil {
		return nil, err
	}

	frameProcessor := processor.NewFrameProcessor(mlClient, logger)

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	wsHandler := handlers.NewWebSocketHandler(frameProcessor, logger)
	streamHandler := handlers.NewStreamHandler(frameProcessor, logger)

	setupRoutes(router, wsHandler, streamHandler)

	return &Server{
		router:         router,
		logger:         logger,
		frameProcessor: frameProcessor,
		mlClient:       mlClient,
	}, nil
}

func setupRoutes(router *gin.Engine, wsHandler *handlers.WebSocketHandler, streamHandler *handlers.StreamHandler) {
	router.GET("/ws", wsHandler.HandleWebSocket)

	api := router.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":    "healthy",
				"timestamp": time.Now(),
				"service":   "motorcycle-feedback-system",
			})
		})

		api.POST("/analyze-frame", streamHandler.ProcessFrame)

		api.GET("/stats", streamHandler.GetStats)
	}

	router.Static("/static", "./client")
	router.StaticFile("/", "./client/index.html")
}
