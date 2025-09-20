package handlers

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/san-kum/motorcycle-cv/server/models"
	"github.com/san-kum/motorcycle-cv/server/processor"
	"go.uber.org/zap"
)

type StreamHandler struct {
	processor *processor.FrameProcessor
	logger    *zap.Logger
	stats     *SystemStats
}

type SystemStats struct {
	TotalFrames    int64     `json:"total_frames"`
	ProcessedOK    int64     `json:"processed_ok"`
	ProcessedError int64     `json:"processed_error"`
	AvgProcessTime float64   `json:"avg_process_time_ms"`
	LastUpdated    time.Time `json:"last_updated"`
	ActiveClients  int       `json:"active_clients"`
}

type FrameUploadRequest struct {
	ImageData string `json:"image_data" binding:"required"`
	Timestamp int64  `json:"timestamp"`
}

func NewStreamHandler(processor *processor.FrameProcessor, logger *zap.Logger) *StreamHandler {
	return &StreamHandler{
		processor: processor,
		logger:    logger,
		stats: &SystemStats{
			LastUpdated: time.Now(),
		},
	}
}

func (h *StreamHandler) ProcessFrame(c *gin.Context) {
	startTime := time.Now()
	h.stats.TotalFrames++

	var request FrameUploadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.logger.Error("Invalid request format", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		h.stats.ProcessedError++
		return
	}

	imageData, err := h.extractImageData(request.ImageData)
	if err != nil {
		h.logger.Error("Failed to process image data", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image data"})
		h.stats.ProcessedError++
		return
	}

	frameRequest := &models.FrameRequest{
		ImageData: imageData,
		Timestamp: request.Timestamp,
		ClientID:  c.ClientIP(),
	}

	result, err := h.processor.ProcessFrame(frameRequest)
	if err != nil {
		h.logger.Error("Frame processing failed",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Processing failed"})
		h.stats.ProcessedError++
		return
	}

	processingTime := time.Since(startTime)
	h.updateProcessingStats(processingTime)
	h.stats.ProcessedOK++

	response := gin.H{
		"analysis":        result,
		"processing_time": processingTime.Milliseconds(),
		"timestamp":       time.Now().Unix(),
	}

	c.JSON(http.StatusOK, response)
}

func (h *StreamHandler) GetStats(c *gin.Context) {
	h.stats.LastUpdated = time.Now()

	var successRate float64
	if h.stats.TotalFrames > 0 {
		successRate = float64(h.stats.ProcessedOK) / float64(h.stats.TotalFrames) * 100
	}

	processorStats := h.processor.GetStats()

	response := gin.H{
		"system":    h.stats,
		"processor": processorStats,
		"metrics": gin.H{
			"success_rate":   successRate,
			"error_rate":     float64(h.stats.ProcessedError) / float64(h.stats.TotalFrames) * 100,
			"uptime_seconds": time.Since(processorStats.StartTime).Seconds(),
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *StreamHandler) UploadVideo(c *gin.Context) {
	file, header, err := c.Request.FormFile("video")
	if err != nil {
		h.logger.Error("Failed to get uploaded file", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	if !h.isValidVideoFile(header.Filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type"})
		return
	}

	if header.Size > 100*1024*1024 { // 100MB limit
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large (max 100MB)"})
		return
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		h.logger.Error("Failed to read uploaded file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process file"})
		return
	}

	jobID := h.processor.CreateVideoJob(fileData, header.Filename, c.ClientIP())

	c.JSON(http.StatusAccepted, gin.H{
		"job_id":  jobID,
		"message": "Video upload successful, processing started",
		"status":  "processing",
	})
}

func (h *StreamHandler) GetVideoJobStatus(c *gin.Context) {
	jobID := c.Param("job_id")

	status, err := h.processor.GetJobStatus(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	c.JSON(http.StatusOK, status)
}

func (h *StreamHandler) extractImageData(dataURL string) ([]byte, error) {
	if !strings.Contains(dataURL, ",") {
		return nil, fmt.Errorf("invalid data URL format")
	}

	parts := strings.Split(dataURL, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid data URL format")
	}

	imageData, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	return imageData, nil
}

func (h *StreamHandler) isValidVideoFile(filename string) bool {
	validExtensions := []string{".mp4", ".avi", ".mov", ".mkv", ".webm"}

	filename = strings.ToLower(filename)
	for _, ext := range validExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

func (h *StreamHandler) updateProcessingStats(duration time.Duration) {
	currentTime := float64(duration.Milliseconds())

	if h.stats.AvgProcessTime == 0 {
		h.stats.AvgProcessTime = currentTime
	} else {
		alpha := 0.1 
		h.stats.AvgProcessTime = alpha*currentTime + (1-alpha)*h.stats.AvgProcessTime
	}
}
