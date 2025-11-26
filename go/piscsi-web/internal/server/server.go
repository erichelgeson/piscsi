package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/piscsi/piscsi-web/internal/auth"
	"github.com/piscsi/piscsi-web/internal/config"
	"github.com/piscsi/piscsi-web/internal/driveprops"
	"github.com/piscsi/piscsi-web/internal/piscsi"
	pb "github.com/piscsi/piscsi-web/proto"
)

// Server represents the HTTP server
type Server struct {
	config       *config.Config
	router       *gin.Engine
	piscsiClient interface {
		SendCommand(cmd *pb.PbCommand) (*pb.PbResult, error)
	}
	sessionStore  *sessions.CookieStore
	driveProps    *driveprops.Properties
	authenticator interface {
		AuthenticateAndAuthorize(username, password string) error
	}
	logger *slog.Logger
}

// New creates a new server instance
func New(cfg *config.Config, logger *slog.Logger) *Server {
	// Set Gin mode based on environment
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// Load HTML templates
	router.LoadHTMLGlob(cfg.TemplatesDir + "/*.html")

	// Add middleware
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))

	// Create PiSCSI client
	piscsiClient := piscsi.NewClient(cfg.PiscsiHost, cfg.PiscsiPort)

	// Create session store
	sessionStore := sessions.NewCookieStore([]byte(cfg.SessionKey))
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   cfg.SessionMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	// Load drive properties
	drivePropsPath := "drive_properties.json"
	driveProps, err := driveprops.LoadProperties(drivePropsPath)
	if err != nil {
		logger.Warn("Failed to load drive properties", "error", err, "path", drivePropsPath)
		// Continue without drive properties - endpoints will return errors
	} else {
		logger.Info("Loaded drive properties", "path", drivePropsPath)
	}

	// Create authenticator (PAM or fallback)
	var authenticator interface {
		AuthenticateAndAuthorize(username, password string) error
	}

	if auth.IsAuthenticationAvailable() {
		logger.Info("Using PAM authentication")
		authenticator = auth.NewAuthenticator("login", cfg.AuthGroup)
	} else {
		logger.Warn("PAM not available, using fallback authentication")
		authenticator = auth.NewFallbackAuthenticator()
	}

	server := &Server{
		config:        cfg,
		router:        router,
		piscsiClient:  piscsiClient,
		sessionStore:  sessionStore,
		driveProps:    driveProps,
		authenticator: authenticator,
		logger:        logger,
	}

	// Setup routes
	server.setupRoutes()

	return server
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Serve static files
	s.router.Static("/static", s.config.StaticDir)

	// Public routes (no authentication required)
	s.router.GET("/login", s.handleLoginPage)
	s.router.POST("/login", s.handleLogin)
	s.router.GET("/healthcheck", s.handleHealthcheck)

	// Protected routes (authentication required)
	protected := s.router.Group("/")
	protected.Use(s.authMiddleware())
	{
		// Page routes
		protected.GET("/", s.handleIndex)
		protected.GET("/logout", s.handleLogout)

		// API routes - Device management
		protected.GET("/env", s.handleEnv)
		protected.POST("/scsi/attach", s.handleAttach)
		protected.POST("/scsi/detach", s.handleDetach)
		protected.POST("/scsi/detach_all", s.handleDetachAll)
		protected.POST("/scsi/eject", s.handleEject)
		protected.POST("/scsi/info", s.handleScsiInfo)
		protected.POST("/scsi/reserve", s.handleScsiReserve)
		protected.POST("/scsi/release", s.handleScsiRelease)

		// API routes - File management
		protected.GET("/files/list", s.handleFilesList)
		protected.POST("/files/upload", s.handleFilesUpload)
		protected.POST("/files/uploadform/", s.handleFilesUploadForm)
		protected.GET("/files/download_image", s.handleFilesDownload)
		protected.POST("/files/delete", s.handleFilesDelete)
		protected.POST("/files/rename", s.handleFilesRename)
		protected.POST("/files/copy", s.handleFilesCopy)
		protected.POST("/files/download_config", s.handleFilesDownloadConfig)
		protected.POST("/files/create", s.handleFilesCreate)
		protected.POST("/files/download_url", s.handleFilesDownloadURL)
		protected.POST("/files/create_iso", s.handleFilesCreateISO)
		protected.POST("/files/extract_image", s.handleFilesExtractImage)

		// API routes - Configuration
		protected.GET("/config/list", s.handleConfigList)
		protected.POST("/config/save", s.handleConfigSave)
		protected.POST("/config/load", s.handleConfigLoad)
		protected.POST("/config/action", s.handleConfigAction)

		// API routes - System operations
		protected.POST("/logs/level", s.handleLogsLevel)
		protected.POST("/logs/show", s.handleLogsShow)
		protected.POST("/sys/rename", s.handleSysRename)
		protected.POST("/sys/reboot", s.handleSysReboot)
		protected.POST("/sys/shutdown", s.handleSysShutdown)

		// API routes - Drive operations
		protected.POST("/drive/create", s.handleDriveCreate)
		protected.POST("/drive/cdrom", s.handleDriveCdrom)

		// Page routes
		protected.GET("/drive/list", s.handleDriveList)
		protected.GET("/sys/admin", s.handleSysAdmin)
		protected.GET("/upload", s.handleUploadPage)
		protected.POST("/files/diskinfo", s.handleFilesDiskinfo)
		protected.GET("/sys/manpage", s.handleSysManpage)

		// Settings endpoints
		protected.POST("/language", s.handleLanguage)
		protected.GET("/theme", s.handleTheme)
		protected.POST("/theme", s.handleTheme)

		// PWA resources
		protected.GET("/pwa/*pwa_path", s.handlePWA)
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	address := fmt.Sprintf("%s:%d", s.config.ServerHost, s.config.ServerPort)
	s.logger.Info("Starting PiSCSI web server", "address", address)
	return s.router.Run(address)
}

// requestLogger logs HTTP requests
func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()

		logger.Info("HTTP request",
			"method", method,
			"path", path,
			"status", statusCode,
			"latency", latency,
			"ip", c.ClientIP(),
		)
	}
}
