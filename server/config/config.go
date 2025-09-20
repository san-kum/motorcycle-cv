package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	ML       MLConfig       `json:"ml"`
	Security SecurityConfig `json:"security"`
	Database DatabaseConfig `json:"database"`
	Redis    RedisConfig    `json:"redis"`
	Logging  LoggingConfig  `json:"logging"`
}

type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
	Environment  string        `json:"environment"`
}

type MLConfig struct {
	BaseURL             string        `json:"base_url"`
	Timeout             time.Duration `json:"timeout"`
	MaxRetries          int           `json:"max_retries"`
	RetryDelay          time.Duration `json:"retry_delay"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
}

type SecurityConfig struct {
	JWTSecretKey   string        `json:"jwt_secret_key"`
	AllowedOrigins []string      `json:"allowed_origins"`
	RateLimitRPS   int           `json:"rate_limit_rps"`
	RateLimitBurst int           `json:"rate_limit_burst"`
	MaxRequestSize int64         `json:"max_request_size"`
	RequestTimeout time.Duration `json:"request_timeout"`
	EnableHTTPS    bool          `json:"enable_https"`
	CertFile       string        `json:"cert_file"`
	KeyFile        string        `json:"key_file"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"db_name"`
	SSLMode  string `json:"ssl_mode"`
	MaxConns int    `json:"max_connections"`
	MinConns int    `json:"min_connections"`
}

type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	PoolSize int    `json:"pool_size"`
}

type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	Output     string `json:"output"`
	MaxSize    int    `json:"max_size"`
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"`
}

func LoadConfig() *Config {
	config := &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnvAsInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvAsDuration("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout: getEnvAsDuration("SERVER_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:  getEnvAsDuration("SERVER_IDLE_TIMEOUT", 60*time.Second),
			Environment:  getEnv("ENVIRONMENT", "development"),
		},
		ML: MLConfig{
			BaseURL:             getEnv("ML_BASE_URL", "http://localhost:5000"),
			Timeout:             getEnvAsDuration("ML_TIMEOUT", 30*time.Second),
			MaxRetries:          getEnvAsInt("ML_MAX_RETRIES", 3),
			RetryDelay:          getEnvAsDuration("ML_RETRY_DELAY", 1*time.Second),
			HealthCheckInterval: getEnvAsDuration("ML_HEALTH_CHECK_INTERVAL", 30*time.Second),
		},
		Security: SecurityConfig{
			JWTSecretKey:   getEnv("JWT_SECRET_KEY", ""),
			AllowedOrigins: getEnvAsStringSlice("ALLOWED_ORIGINS", []string{"*"}),
			RateLimitRPS:   getEnvAsInt("RATE_LIMIT_RPS", 100),
			RateLimitBurst: getEnvAsInt("RATE_LIMIT_BURST", 200),
			MaxRequestSize: getEnvAsInt64("MAX_REQUEST_SIZE", 10*1024*1024), // 10MB
			RequestTimeout: getEnvAsDuration("REQUEST_TIMEOUT", 30*time.Second),
			EnableHTTPS:    getEnvAsBool("ENABLE_HTTPS", false),
			CertFile:       getEnv("CERT_FILE", ""),
			KeyFile:        getEnv("KEY_FILE", ""),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "motorcycle_cv"),
			SSLMode:  getEnv("DB_SSL_MODE", "disable"),
			MaxConns: getEnvAsInt("DB_MAX_CONNS", 25),
			MinConns: getEnvAsInt("DB_MIN_CONNS", 5),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvAsInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
			PoolSize: getEnvAsInt("REDIS_POOL_SIZE", 10),
		},
		Logging: LoggingConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "json"),
			Output:     getEnv("LOG_OUTPUT", "stdout"),
			MaxSize:    getEnvAsInt("LOG_MAX_SIZE", 100),
			MaxBackups: getEnvAsInt("LOG_MAX_BACKUPS", 3),
			MaxAge:     getEnvAsInt("LOG_MAX_AGE", 28),
		},
	}

	return config
}

func (c *Config) ValidateConfig(logger *zap.Logger) error {
	var errors []string

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errors = append(errors, "server port must be between 1 and 65535")
	}

	if c.ML.BaseURL == "" {
		errors = append(errors, "ML base URL is required")
	}

	if c.Security.JWTSecretKey == "" {
		logger.Warn("JWT secret key not set, using random key")
	}

	if c.Security.MaxRequestSize <= 0 {
		errors = append(errors, "max request size must be positive")
	}

	if c.Database.Host == "" {
		errors = append(errors, "database host is required")
	}

	if c.Database.Port < 1 || c.Database.Port > 65535 {
		errors = append(errors, "database port must be between 1 and 65535")
	}

	if c.Redis.Host == "" {
		errors = append(errors, "Redis host is required")
	}

	if c.Redis.Port < 1 || c.Redis.Port > 65535 {
		errors = append(errors, "Redis port must be between 1 and 65535")
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errors, ", "))
	}

	return nil
}


func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvAsStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}
