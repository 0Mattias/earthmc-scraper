package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the worker.
type Config struct {
	// Database
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBPoolMax  int

	// Cloud SQL
	CloudSQLConnectionName string

	// Scraper intervals
	HighFreqInterval time.Duration
	LowFreqInterval  time.Duration

	// HTTP server
	Port int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	c := &Config{
		DBHost:                 getEnv("DB_HOST", "localhost"),
		DBPort:                 getEnvInt("DB_PORT", 5432),
		DBName:                 getEnv("DB_NAME", "earthmc"),
		DBUser:                 getEnv("DB_USER", "earthmc_worker"),
		DBPassword:             getEnv("DB_PASSWORD", ""),
		DBPoolMax:              getEnvInt("DB_POOL_MAX", 10),
		CloudSQLConnectionName: getEnv("CLOUD_SQL_CONNECTION_NAME", ""),
		Port:                   getEnvInt("PORT", 8080),
	}

	var err error
	c.HighFreqInterval, err = time.ParseDuration(getEnv("HIGH_FREQ_INTERVAL", "3s"))
	if err != nil {
		return nil, fmt.Errorf("invalid HIGH_FREQ_INTERVAL: %w", err)
	}

	c.LowFreqInterval, err = time.ParseDuration(getEnv("LOW_FREQ_INTERVAL", "3m"))
	if err != nil {
		return nil, fmt.Errorf("invalid LOW_FREQ_INTERVAL: %w", err)
	}

	if c.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD is required")
	}

	return c, nil
}

// DSN returns the PostgreSQL connection string.
func (c *Config) DSN() string {
	if c.CloudSQLConnectionName != "" {
		// Cloud SQL Unix socket path
		return fmt.Sprintf("host=/cloudsql/%s user=%s password=%s dbname=%s sslmode=disable pool_max_conns=%d",
			c.CloudSQLConnectionName, c.DBUser, c.DBPassword, c.DBName, c.DBPoolMax)
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable pool_max_conns=%d",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBPoolMax)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
