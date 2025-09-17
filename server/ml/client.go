package ml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/san-kum/motorcycle-cv/server/models"
	"go.uber.org/zap"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	config     *ClientConfig
}

type ClientConfig struct {
	Timeout             time.Duration
	MaxRetries          int
	RetryDelay          time.Duration
	HealthCheckInterval time.Duration
}

type AnalysisRequest struct {
	ImageData []byte                 `json:"image_data"`
	Timestamp int64                  `json:"timestamp"`
	Config    map[string]interface{} `json:"config,omitempty"`
}

type AnalysisResponse struct {
	OverallScore   int                 `json:"overall_score"`
	PostureScore   int                 `json:"posture_score"`
	LaneScore      int                 `json:"lane_score"`
	SpeedScore     int                 `json:"speed_score"`
	Detections     []ObjectDetection   `json:"detections"`
	PoseKeypoints  []PoseKeypoint      `json:"pose_keypoints"`
	SceneAnalysis  SceneAnalysisResult `json:"scene_analysis"`
	ProcessingTime float64             `json:"processing_time"`
	ModelVersion   string              `json:"model_version"`
}

type ObjectDetection struct {
	Class       string  `json:"class"`
	Confidence  float64 `json:"confidence"`
	BoundingBox BBox    `json:"bounding_box"`
	TrackID     int     `json:"track_id,omitempty"`
}

type BBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type PoseKeypoint struct {
	Name       string  `json:"name"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Confidence float64 `json:"confidence"`
	Visible    bool    `json:"visible"`
}

type SceneAnalysisResult struct {
	RoadType         string  `json:"road_type"`
	LaneCount        int     `json:"lane_count"`
	WeatherCondition string  `json:"weather_condition"`
	TimeOfDay        string  `json:"time_of_day"`
	TrafficDensity   string  `json:"traffic_density"`
	RoadCondition    string  `json:"road_condition"`
	SpeedLimit       int     `json:"speed_limit"`
	Visibility       float64 `json:"visibility"`
}

func NewClient(baseURL string, logger *zap.Logger) (*Client, error) {
	config := &ClientConfig{
		Timeout:             30 * time.Second,
		MaxRetries:          3,
		RetryDelay:          1 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}

	client := &Client{
		baseURL: baseURL,
		logger:  logger,
		config:  config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: true,
			},
		},
	}

	if err := client.HealthCheck(); err != nil {
		logger.Warn("ML service not available at startup", zap.Error(err))
	}

	go client.startHealthChecker()

	return client, nil
}

func (c *Client) AnalyzeFrame(request *models.FrameRequest) (*models.AnalysisResult, error) {
	mlRequest := &AnalysisRequest{
		ImageData: request.ImageData,
		Timestamp: request.Timestamp,
		Config: map[string]interface{}{
			"client_id": request.ClientID,
		},
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Warn("Retrying ML analysis request",
				zap.Int("attempt", attempt),
				zap.Error(lastErr))
			time.Sleep(c.config.RetryDelay * time.Duration(attempt))
		}

		result, err := c.executeAnalysisRequest(mlRequest)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("ML analysis failed after %d attempts: %w",
		c.config.MaxRetries, lastErr)
}

func (c *Client) executeAnalysisRequest(request *AnalysisRequest) (*models.AnalysisResult, error) {
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/analyze", c.baseURL)
	httpRequest, err := http.NewRequest("POST", url, bytes.NewBuffer(requestData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("User-Agent", "motorcycle-feedback-system/1.0")

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("ML service error (status %d): %s",
			response.StatusCode, string(bodyBytes))
	}

	var mlResponse AnalysisResponse
	if err := json.NewDecoder(response.Body).Decode(&mlResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return c.convertMLResponse(&mlResponse), nil
}

func (c *Client) convertMLResponse(mlResp *AnalysisResponse) *models.AnalysisResult {
	result := &models.AnalysisResult{
		OverallScore:   mlResp.OverallScore,
		PostureScore:   mlResp.PostureScore,
		LaneScore:      mlResp.LaneScore,
		SpeedScore:     mlResp.SpeedScore,
		ProcessingTime: mlResp.ProcessingTime,
		ModelVersion:   mlResp.ModelVersion,
		Timestamp:      time.Now().Unix(),
	}

	var annotations []models.Annotation
	for _, detection := range mlResp.Detections {
		annotations = append(annotations, models.Annotation{
			Type:       "bounding_box",
			X:          detection.BoundingBox.X,
			Y:          detection.BoundingBox.Y,
			Width:      detection.BoundingBox.Width,
			Height:     detection.BoundingBox.Height,
			Label:      detection.Class,
			Confidence: detection.Confidence,
		})
	}

	for _, keypoint := range mlResp.PoseKeypoints {
		if keypoint.Visible && keypoint.Confidence > 0.5 {
			annotations = append(annotations, models.Annotation{
				Type:       "keypoint",
				X:          keypoint.X,
				Y:          keypoint.Y,
				Width:      5, // Small circle for keypoint
				Height:     5,
				Label:      keypoint.Name,
				Confidence: keypoint.Confidence,
			})
		}
	}

	result.Annotations = annotations

	result.Metadata = map[string]interface{}{
		"road_type":         mlResp.SceneAnalysis.RoadType,
		"lane_count":        mlResp.SceneAnalysis.LaneCount,
		"weather_condition": mlResp.SceneAnalysis.WeatherCondition,
		"time_of_day":       mlResp.SceneAnalysis.TimeOfDay,
		"traffic_density":   mlResp.SceneAnalysis.TrafficDensity,
		"road_condition":    mlResp.SceneAnalysis.RoadCondition,
		"speed_limit":       mlResp.SceneAnalysis.SpeedLimit,
		"visibility":        mlResp.SceneAnalysis.Visibility,
	}

	return result
}

func (c *Client) HealthCheck() error {
	url := fmt.Sprintf("%s/health", c.baseURL)

	response, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("ML service unhealthy (status %d)", response.StatusCode)
	}

	return nil
}

func (c *Client) startHealthChecker() {
	ticker := time.NewTicker(c.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := c.HealthCheck(); err != nil {
			c.logger.Error("ML service health check failed", zap.Error(err))
		} else {
			c.logger.Debug("ML service health check passed")
		}
	}
}

func (c *Client) GetModelInfo() (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/models/info", c.baseURL)

	response, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model info request failed (status %d)", response.StatusCode)
	}

	var modelInfo map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&modelInfo); err != nil {
		return nil, fmt.Errorf("failed to decode model info: %w", err)
	}

	return modelInfo, nil
}

func (c *Client) UpdateConfig(config map[string]interface{}) error {
	requestData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	url := fmt.Sprintf("%s/config", c.baseURL)
	request, err := http.NewRequest("PUT", url, bytes.NewBuffer(requestData))
	if err != nil {
		return fmt.Errorf("failed to create config request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("config update failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("config update failed (status %d)", response.StatusCode)
	}

	c.logger.Info("ML service configuration updated successfully")
	return nil
}
