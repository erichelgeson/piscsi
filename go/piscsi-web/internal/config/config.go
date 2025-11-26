package config

import (
	"os"
	"strconv"
)

// Config holds application configuration
type Config struct {
	// Server configuration
	ServerPort int
	ServerHost string

	// PiSCSI daemon configuration
	PiscsiHost string
	PiscsiPort int

	// File paths
	BaseDir       string // Base directory for image files
	SharedDir     string // Shared files directory
	ConfigDir     string // Configuration files directory
	TemplatesDir  string // Templates directory
	StaticDir     string // Static assets directory

	// File size limits
	MaxFileSize int64 // Maximum upload file size in bytes

	// Authentication
	AuthGroup string // PAM group for authentication

	// Session
	SessionKey    string // Secret key for session encryption
	SessionMaxAge int    // Session max age in seconds
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		ServerPort:    8080,
		ServerHost:    "0.0.0.0",
		PiscsiHost:    "localhost",
		PiscsiPort:    6868,
		BaseDir:       getEnv("BASE_DIR", "/home/pi/images"),
		SharedDir:     getEnv("SHARED_DIR", "/home/pi/shared_files"),
		ConfigDir:     getEnv("CONFIG_DIR", "/home/pi/.config/piscsi"),
		TemplatesDir:  "web/templates",
		StaticDir:     "web/static",
		MaxFileSize:   getEnvInt64("MAX_FILE_SIZE", 4*1024*1024*1024), // 4GB default
		AuthGroup:     getEnv("AUTH_GROUP", "piscsi"),
		SessionKey:    getEnv("SESSION_KEY", "piscsi-session-key-change-me"),
		SessionMaxAge: getEnvInt("SESSION_MAX_AGE", 86400), // 24 hours
	}
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an environment variable as int or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvInt64 gets an environment variable as int64 or returns a default value
func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}
