package models

import "time"

type FrameRequest struct {
	ImageData []byte         `json:"image_data"`
	Timestamp int64          `json:"timestamp"`
	ClientID  string         `json:"client_id"`
	Metadata  map[string]any `json:"metadata"`
}

type AnalysisResult struct {
	OverallScore   int            `json:"overall_score"`
	PostureScore   int            `json:"posture_score"`
	LaneScore      int            `json:"lane_score"`
	SpeedScore     int            `json:"speed_score"`
	Annotations    []Annotation   `json:"annotations"`
	Feedback       []Feedback     `json:"feedback"`
	Metadata       map[string]any `json:"metadata"`
	ProcessingTime float64        `json:"processing_time"`
	ModelVersion   string         `json:"model_version"`
	Timestamp      int64          `json:"timestamp"`
}

type Annotation struct {
	Type       string  `json:"type"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Points     []Point `json:"points"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Color      string  `json:"color"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Feedback struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Score    int    `json:"score"`
	Category string `json:"category"`
	Priority int    `json:"priority"`
}

type RidingSession struct {
	ID              string         `json:"id"`
	ClientID        string         `json:"client_id"`
	StartTime       time.Time      `json:"start_time"`
	EndTime         time.Time      `json:"end_time"`
	TotalFrames     int            `json:"total_frames"`
	AvgOverallScore int            `json:"avg_overall_score"`
	AvgPostureScore int            `json:"avg_posture_score"`
	AvgLaneScore    int            `json:"avg_lane_score"`
	AvgSpeedScore   int            `json:"avg_speed_score"`
	Highlights      []Highlight    `json:"highlights"`
	Summary         SessionSummary `json:"summary"`
}

type Highlight struct {
	Timestamp   int64  `json:"timestamp"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Score       int    `json:"score"`
	Category    string `json:"category"`
}

type SessionSummary struct {
	OverallGrade       string   `json:"overall_grade"`
	StrengthAreas      []string `json:"strength_areas"`
	ImprovementAreas   []string `json:"improvement_areas"`
	SafetyScore        int      `json:"safety_score"`
	RecommendedActions []string `json:"recommended_actions"`
	TotalDistance      float64  `json:"total_distance_km"`
	Duration           float64  `json:"duration_minutes"`
}
type DetectionClass string

const (
	ClassMotorcycle   DetectionClass = "motorcycle"
	ClassRider        DetectionClass = "rider"
	ClassVehicle      DetectionClass = "vehicle"
	ClassPedestrian   DetectionClass = "pedestrian"
	ClassTrafficLight DetectionClass = "traffic_light"
	ClassRoadSign     DetectionClass = "road_sign"
	ClassLane         DetectionClass = "lane"
	ClassObstacle     DetectionClass = "obstacle"
	ClassAnimal       DetectionClass = "animal"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type SafetyAlert struct {
	Type           string    `json:"type"`
	Level          RiskLevel `json:"level"`
	Message        string    `json:"message"`
	Timestamp      int64     `json:"timestamp"`
	Location       Point     `json:"location"`
	Severity       int       `json:"severity"`
	ActionRequired string    `json:"action_required"`
}

type PerformanceMetrics struct {
	SpeedConsistency   float64 `json:"speed_consistency"`
	LaneStability      float64 `json:"lane_stability"`
	PostureConsistency float64 `json:"posture_consistency"`
	ReactionTime       float64 `json:"reaction_time"`
	SmoothnessFactor   float64 `json:"smoothness_factor"`
	SafetyMargin       float64 `json:"safety_margin"`
}

type WeatherConditions struct {
	Temperature   float64 `json:"temperature"`
	Humidity      float64 `json:"humidity"`
	WindSpeed     float64 `json:"wind_speed"`
	Precipitation string  `json:"precipitation"`  // none, light, moderate, heavy
	Visibility    string  `json:"visibility"`     // excellent, good, moderate, poor
	RoadCondition string  `json:"road_condition"` // dry, wet
}

type TrafficAnalytics struct {
	VehicleCount     int     `json:"vehicle_count"`
	AverageSpeed     float64 `json:"average_speed"`
	TrafficDensity   string  `json:"traffic_density"`
	NearestVehicle   float64 `json:"nearest_vehicle"`
	OvertakingEvents int     `json:"overtaking_events"`
	LaneChanges      int     `json:"lane_changes"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

type APIResponse struct {
	Success bool          `json:"success"`
	Data    any           `json:"data"`
	Error   *APIError     `json:"error"`
	Meta    *ResponseMeta `json:"meta"`
}

type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type ResponseMeta struct {
	RequestID      string    `json:"request_id"`
	Timestamp      time.Time `json:"timestamp"`
	ProcessingTime float64   `json:"processing_time"`
	Version        string    `json:"version"`
}
