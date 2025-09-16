package processor

import (
	"crypto/md5"
	"fmt"
	"sync"
	"time"

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
}

type ProcessorStats struct {
	StartTime             time.Time `json:"start_time"`
	TotalProcessed        int64     `json:"total_processed"`
	SuccessfullyProcessed int64     `json:"successfully_processed"`
	FailedProcessed       int64     `json:"failed_processed"`
	AverageLatency        float64   `json:"average_latency"`
	QueueSize             int       `json:"queue_size"`
	ActiveWorkers         int       `json:"active_workers"`
}

type ProcessorConfig struct {
	MaxQueueSize        int     `json:"max_queue_size"`
	MaxWorkers          int     `json:"max_workers"`
	ProcessingTimeout   int     `json:"processing_timeout"`
	SkipSimilarFrames   bool    `json:"skip_similar_frames"`
	SimilarityThreshold float64 `json:"similarity_threshold"`
}

type VideoJob struct {
	ID        string                  `json:"id"`
	Filename  string                  `json:"filename"`
	Status    string                  `json:"status"`
	Progress  float64                 `json:"progress"`
	StartTime time.Time               `json:"start_time"`
	Results   []models.AnalysisResult `json:"results"`
	Error     string                  `json:"error"`
}

func NewFrameProcessor(mlClient *ml.Client, logger *zap.Logger) *FrameProcessor {
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

	processor := &FrameProcessor{
		mlClient:   mlClient,
		logger:     logger,
		stats:      stats,
		config:     config,
		jobTracker: make(map[string]*VideoJob),
	}

	processor.queue = NewProcessingQueue(config.MaxQueueSize, config.MaxWorkers, processor.processFrame)
	return processor
}

func (fp *FrameProcessor) ProcessFrame(request *models.FrameRequest) (*models.AnalysisResult, error) {
	startTime := time.Now()
	fp.stats.TotalProcessed++

	if fp.config.SkipSimilarFrames && fp.isSimilarFrame(request.ImageData) {
		fp.logger.Debug("skipping similar frame")
		return fp.getCachedResult(), nil
	}
	resultChan := make(chan *ProcessingResult, 1)

	QueueItem := &QueueItem{
		Request:    request,
		ResultChan: resultChan,
		StartTime:  startTime,
	}
	if !fp.queue.Enqueue(QueueItem) {
		fp.stats.FailedProcessed++
		return nil, fmt.Errorf("processing queue full")
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

	feedback := fp.generateFeedback(analysis)
	analysis.Feedback = feedback

	fp.cacheResult(item.Request.ImageData, analysis)
	item.ResultChan <- &ProcessingResult{Analysis: analysis}

}

func (fp *FrameProcessor) generateFeedback(analysis *models.AnalysisResult) []models.Feedback {
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

	stats := *fp.stats
	stats.QueueSize = fp.queue.Size()
	return &stats
}

func (fp *FrameProcessor) isSimilarFrame(imageData []byte) bool {
	hash := fmt.Sprintf("%x", md5.Sum(imageData))
	return fp.checkFrameHash(hash)
}

func (fp *FrameProcessor) checkFrameHash(hash string) bool {
	// TODO: Implement proper frame hash caching with LRU eviction
	return false
}

func (fp *FrameProcessor) cacheResult(imageData []byte, result *models.AnalysisResult) {
	// TODO: Implement result caching mechanism
}

func (fp *FrameProcessor) getCachedResult() *models.AnalysisResult {
	// TODO: Implement cached result retrieval
	return &models.AnalysisResult{
		OverallScore: 75,
		PostureScore: 80,
		LaneScore:    70,
		SpeedScore:   75,
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