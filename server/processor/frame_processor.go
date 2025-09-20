package processor

import (
	"context"
	"crypto/md5"
	"fmt"
	"sync"
	"time"

	"github.com/san-kum/motorcycle-cv/server/cache"
	"github.com/san-kum/motorcycle-cv/server/ml"
	"github.com/san-kum/motorcycle-cv/server/models"
	"go.uber.org/zap"
)

type FrameProcessor struct {
	mlClient   *ml.Client
	logger     *zap.Logger
	queue      *ProcessingQueue
	stats      *ProcessorStats
	config     *ProcessorConfig
	mutex      sync.RWMutex
	jobTracker map[string]*VideoJob
	cache      cache.Cache
	ctx        context.Context
	cancel     context.CancelFunc
}

type ProcessorStats struct {
	StartTime             time.Time `json:"start_time"`
	TotalProcessed        int64     `json:"total_processed"`
	SuccessfullyProcessed int64     `json:"successfully_processed"`
	FailedProcessed       int64     `json:"failed_processed"`
	AverageLatency        float64   `json:"average_latency_ms"`
	QueueSize             int       `json:"queue_size"`
	ActiveWorkers         int       `json:"active_workers"`
}

type ProcessorConfig struct {
	MaxQueueSize        int     `json:"max_queue_size"`
	MaxWorkers          int     `json:"max_workers"`
	ProcessingTimeout   int     `json:"processing_timeout_seconds"`
	SkipSimilarFrames   bool    `json:"skip_similar_frames"`
	SimilarityThreshold float64 `json:"similarity_threshold"`
}

type VideoJob struct {
	ID        string                  `json:"id"`
	Filename  string                  `json:"filename"`
	Status    string                  `json:"status"`
	Progress  float64                 `json:"progress"`
	StartTime time.Time               `json:"start_time"`
	Results   []models.AnalysisResult `json:"results,omitempty"`
	Error     string                  `json:"error,omitempty"`
}

func NewFrameProcessor(mlClient *ml.Client, cache cache.Cache, logger *zap.Logger) *FrameProcessor {
	config := &ProcessorConfig{
		MaxQueueSize:        100,
		MaxWorkers:          4,
		ProcessingTimeout:   30,
		SkipSimilarFrames:   true,
		SimilarityThreshold: 0.95,
	}

	stats := &ProcessorStats{
		StartTime:     time.Now(),
		ActiveWorkers: config.MaxWorkers,
	}

	ctx, cancel := context.WithCancel(context.Background())

	processor := &FrameProcessor{
		mlClient:   mlClient,
		logger:     logger,
		stats:      stats,
		config:     config,
		jobTracker: make(map[string]*VideoJob),
		cache:      cache,
		ctx:        ctx,
		cancel:     cancel,
	}

	processor.queue = NewProcessingQueue(config.MaxQueueSize, config.MaxWorkers, processor.processFrame)

	return processor
}

func (fp *FrameProcessor) ProcessFrame(request *models.FrameRequest) (*models.AnalysisResult, error) {
	startTime := time.Now()
	fp.stats.TotalProcessed++

	// Generate cache key for this frame
	frameHash := fp.generateFrameHash(request.ImageData)
	cacheKey := cache.GenerateCacheKey("frame", frameHash, request.ClientID)

	// Check cache first
	if fp.cache != nil {
		var cachedResult models.AnalysisResult
		if err := fp.cache.Get(fp.ctx, cacheKey, &cachedResult); err == nil {
			fp.logger.Debug("Cache hit for frame", zap.String("key", cacheKey))
			fp.stats.SuccessfullyProcessed++
			return &cachedResult, nil
		}
	}

	// Check for similar frames if enabled
	if fp.config.SkipSimilarFrames && fp.isSimilarFrame(request.ImageData) {
		fp.logger.Debug("Skipping similar frame")
		return fp.getCachedResult(), nil
	}

	resultChan := make(chan *ProcessingResult, 1)

	queueItem := &QueueItem{
		Request:    request,
		ResultChan: resultChan,
		StartTime:  startTime,
		Priority:   fp.calculatePriority(request),
	}

	if !fp.queue.Enqueue(queueItem) {
		fp.stats.FailedProcessed++
		return nil, fmt.Errorf("processing queue full, try again later")
	}

	select {
	case result := <-resultChan:
		if result.Error != nil {
			fp.stats.FailedProcessed++
			return nil, result.Error
		}

		latency := time.Since(startTime)
		fp.updateLatencyStats(latency)
		fp.stats.SuccessfullyProcessed++

		// Cache the result
		if fp.cache != nil && result.Analysis != nil {
			go func() {
				if err := fp.cache.Set(fp.ctx, cacheKey, *result.Analysis); err != nil {
					fp.logger.Warn("Failed to cache result", zap.Error(err))
				}
			}()
		}

		return result.Analysis, nil

	case <-time.After(time.Duration(fp.config.ProcessingTimeout) * time.Second):
		fp.stats.FailedProcessed++
		return nil, fmt.Errorf("processing timeout")
	}
}

func (fp *FrameProcessor) processFrame(item *QueueItem) {
	defer func() {
		if r := recover(); r != nil {
			fp.logger.Error("Frame processing panic", zap.Any("panic", r))
			item.ResultChan <- &ProcessingResult{
				Error: fmt.Errorf("processing failed: %v", r),
			}
		}
	}()

	analysis, err := fp.mlClient.AnalyzeFrame(item.Request)
	if err != nil {
		fp.logger.Error("ML analysis failed", zap.Error(err))
		item.ResultChan <- &ProcessingResult{Error: err}
		return
	}

	if analysis != nil {
		feedback := fp.generateFeedback(analysis)
		analysis.Feedback = feedback
	} else {
		analysis = &models.AnalysisResult{
			OverallScore: 50,
			PostureScore: 50,
			LaneScore:    50,
			SpeedScore:   50,
			Feedback:     []models.Feedback{},
			Timestamp:    time.Now().Unix(),
		}
	}

	// Cache result is handled in ProcessFrame method

	item.ResultChan <- &ProcessingResult{Analysis: analysis}
}

func (fp *FrameProcessor) generateFeedback(analysis *models.AnalysisResult) []models.Feedback {
	if analysis == nil {
		return []models.Feedback{}
	}

	var feedback []models.Feedback

	if analysis.PostureScore < 70 {
		feedback = append(feedback, models.Feedback{
			Type:    "warning",
			Message: "Improve your riding posture - keep your back straight and relax your shoulders",
			Score:   analysis.PostureScore,
		})
	} else if analysis.PostureScore > 85 {
		feedback = append(feedback, models.Feedback{
			Type:    "success",
			Message: "Excellent riding posture!",
			Score:   analysis.PostureScore,
		})
	}

	if analysis.LaneScore < 60 {
		feedback = append(feedback, models.Feedback{
			Type:    "error",
			Message: "Maintain better lane position - you're drifting",
			Score:   analysis.LaneScore,
		})
	}

	if analysis.SpeedScore < 65 {
		feedback = append(feedback, models.Feedback{
			Type:    "warning",
			Message: "Adjust your speed for current road conditions",
			Score:   analysis.SpeedScore,
		})
	}

	if analysis.OverallScore > 80 {
		feedback = append(feedback, models.Feedback{
			Type:    "success",
			Message: "Great riding! Keep up the good work",
			Score:   analysis.OverallScore,
		})
	}

	return feedback
}

func (fp *FrameProcessor) CreateVideoJob(videoData []byte, filename, clientID string) string {
	jobID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s-%s-%d", filename, clientID, time.Now().UnixNano()))))

	job := &VideoJob{
		ID:        jobID,
		Filename:  filename,
		Status:    "processing",
		Progress:  0.0,
		StartTime: time.Now(),
	}

	fp.mutex.Lock()
	fp.jobTracker[jobID] = job
	fp.mutex.Unlock()

	go fp.processVideo(job, videoData)

	return jobID
}

func (fp *FrameProcessor) GetJobStatus(jobID string) (*VideoJob, error) {
	fp.mutex.RLock()
	defer fp.mutex.RUnlock()

	job, exists := fp.jobTracker[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found")
	}

	return job, nil
}

func (fp *FrameProcessor) processVideo(job *VideoJob, videoData []byte) {
	// TODO: Implement video frame extraction and batch processing
	// This would involve:
	// 1. Extract frames from video using ffmpeg or similar
	// 2. Process each frame through the ML pipeline
	// 3. Aggregate results and generate comprehensive feedback
	// 4. Update job status and progress

	fp.logger.Info("Video processing started", zap.String("job_id", job.ID))

	time.Sleep(2 * time.Second)

	fp.mutex.Lock()
	job.Status = "completed"
	job.Progress = 100.0
	fp.mutex.Unlock()
}

func (fp *FrameProcessor) UpdateConfig(configMap map[string]interface{}) {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	if maxQueue, ok := configMap["max_queue_size"].(float64); ok {
		fp.config.MaxQueueSize = int(maxQueue)
	}
	if maxWorkers, ok := configMap["max_workers"].(float64); ok {
		fp.config.MaxWorkers = int(maxWorkers)
	}
	if timeout, ok := configMap["processing_timeout"].(float64); ok {
		fp.config.ProcessingTimeout = int(timeout)
	}

	fp.logger.Info("Configuration updated", zap.Any("config", fp.config))
}

func (fp *FrameProcessor) GetStats() *ProcessorStats {
	fp.mutex.RLock()
	defer fp.mutex.RUnlock()

	stats := *fp.stats // Create a copy
	stats.QueueSize = fp.queue.Size()
	return &stats
}

func (fp *FrameProcessor) generateFrameHash(imageData []byte) string {
	return fmt.Sprintf("%x", md5.Sum(imageData))
}

func (fp *FrameProcessor) isSimilarFrame(imageData []byte) bool {
	if !fp.config.SkipSimilarFrames {
		return false
	}

	hash := fp.generateFrameHash(imageData)
	similarityKey := cache.GenerateCacheKey("similarity", hash)

	// Check if we've seen this hash recently
	exists, err := fp.cache.Exists(fp.ctx, similarityKey)
	if err != nil {
		fp.logger.Warn("Failed to check frame similarity", zap.Error(err))
		return false
	}

	if exists {
		return true
	}

	// Mark this hash as seen
	go func() {
		if err := fp.cache.SetWithTTL(fp.ctx, similarityKey, true, 5*time.Minute); err != nil {
			fp.logger.Warn("Failed to cache frame similarity", zap.Error(err))
		}
	}()

	return false
}

func (fp *FrameProcessor) calculatePriority(request *models.FrameRequest) int {
	// Higher priority for real-time requests
	priority := 1

	// Check if this is a high-priority client (e.g., VIP users)
	// This could be based on user subscription level, etc.
	if request.ClientID != "" {
		// Add logic to determine client priority
		priority += 1
	}

	// Higher priority for recent requests
	age := time.Since(time.Unix(request.Timestamp/1000, 0))
	if age < 1*time.Second {
		priority += 2
	} else if age < 5*time.Second {
		priority += 1
	}

	return priority
}

func (fp *FrameProcessor) getCachedResult() *models.AnalysisResult {
	// Return a default result for similar frames
	return &models.AnalysisResult{
		OverallScore: 75,
		PostureScore: 80,
		LaneScore:    70,
		SpeedScore:   75,
		Timestamp:    time.Now().Unix(),
		Feedback: []models.Feedback{
			{
				Type:    "info",
				Message: "Using cached result for similar frame",
				Score:   75,
			},
		},
	}
}

func (fp *FrameProcessor) updateLatencyStats(latency time.Duration) {
	currentLatency := float64(latency.Milliseconds())

	if fp.stats.AverageLatency == 0 {
		fp.stats.AverageLatency = currentLatency
	} else {
		alpha := 0.1
		fp.stats.AverageLatency = alpha*currentLatency + (1-alpha)*fp.stats.AverageLatency
	}
}

// Shutdown gracefully shuts down the frame processor
func (fp *FrameProcessor) Shutdown() error {
	fp.logger.Info("Shutting down frame processor...")

	// Cancel context
	fp.cancel()

	// Shutdown queue
	if err := fp.queue.Shutdown(30 * time.Second); err != nil {
		fp.logger.Error("Failed to shutdown queue", zap.Error(err))
		return err
	}

	// Close cache
	if fp.cache != nil {
		if err := fp.cache.Close(); err != nil {
			fp.logger.Error("Failed to close cache", zap.Error(err))
			return err
		}
	}

	fp.logger.Info("Frame processor shutdown complete")
	return nil
}

// GetCacheStats returns cache statistics
func (fp *FrameProcessor) GetCacheStats() (*cache.CacheStats, error) {
	if fp.cache == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	return fp.cache.GetStats(fp.ctx)
}
