package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/piscsi/piscsi-web/internal/piscsi"
	pb "github.com/piscsi/piscsi-web/proto"
	"github.com/piscsi/piscsi-web/web"
)

// handleIndex serves the main control page
func (s *Server) handleIndex(c *gin.Context) {
	// Get base template data
	data := s.getBaseTemplateData(c)

	// Get device list
	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ListDevices())

	var devices []map[string]interface{}
	showUnits := false

	if err == nil && result.GetStatus() {
		// Build device list
		for id := 0; id <= 7; id++ {
			for unit := 0; unit <= 31; unit++ {
				device := map[string]interface{}{
					"ID":              id,
					"Unit":            unit,
					"DeviceName":      "",
					"DeviceType":      "",
					"File":            "",
					"Vendor":          "",
					"Product":         "",
					"Revision":        "",
					"Reserved":        false,
					"Occupied":        false,
					"NoMedia":         false,
					"Removable":       false,
					"ReservationMemo": "",
				}

				// Check if device is attached
				for _, dev := range result.GetDevicesInfo().GetDevices() {
					if int(dev.GetId()) == id && int(dev.GetUnit()) == unit {
						device["Occupied"] = true
						deviceType := dev.GetType().String()
						device["DeviceName"] = deviceType
						device["DeviceType"] = strings.ToLower(deviceType)
						if dev.GetFile() != nil {
							device["File"] = dev.GetFile().GetName()
						}
						device["Vendor"] = dev.GetVendor()
						device["Product"] = dev.GetProduct()
						device["Revision"] = dev.GetRevision()
						if dev.GetProperties() != nil {
							device["Removable"] = dev.GetProperties().GetRemovable()
						}
						if dev.GetStatus() != nil {
							device["NoMedia"] = dev.GetStatus().GetRemoved()
						}
						if unit > 0 {
							showUnits = true
						}
						break
					}
				}

				// Only add device if it's ID 0-7 and either occupied or unit 0
				if unit == 0 || device["Occupied"].(bool) {
					devices = append(devices, device)
				}
			}
		}
	}

	// Get file list
	files, filesBySubdir := s.getImageFiles()

	// Check if directories exist
	configDirExists := false
	imageDirExists := false
	if info, err := os.Stat(s.config.ConfigDir); err == nil && info.IsDir() {
		configDirExists = true
	}
	if info, err := os.Stat(s.config.BaseDir); err == nil && info.IsDir() {
		imageDirExists = true
	}

	// Get config files
	var configFiles []string
	if configDirExists {
		entries, err := os.ReadDir(s.config.ConfigDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
					configFiles = append(configFiles, strings.TrimSuffix(entry.Name(), ".json"))
				}
			}
		}
	}

	// Get free disk space
	freeDiskSpace := 0

	// Valid SCSI IDs (0-7)
	validScsiIds := []int{0, 1, 2, 3, 4, 5, 6, 7}
	recommendedScsiId := 6

	// Device types
	deviceTypes := []map[string]string{
		{"Key": "SCHD", "Name": "SCSI Hard Disk"},
		{"Key": "SCCD", "Name": "SCSI CD-ROM"},
		{"Key": "SCRM", "Name": "SCSI Removable Media"},
		{"Key": "SCMO", "Name": "SCSI Magneto-Optical"},
	}

	// Valid image suffixes
	validImageSuffixes := []string{"hda", "hds", "hdi", "hdf", "iso", "cdr"}

	data["ConfigDir"] = s.config.ConfigDir
	data["ConfigDirExists"] = configDirExists
	data["ConfigFiles"] = configFiles
	data["Devices"] = devices
	data["ShowUnits"] = showUnits
	data["Files"] = files
	data["FilesBySubdir"] = filesBySubdir
	data["ImageDir"] = s.config.BaseDir
	data["ImageDirExists"] = imageDirExists
	data["ImageRootDir"] = s.config.BaseDir
	data["ValidScsiIds"] = validScsiIds
	data["RecommendedScsiId"] = recommendedScsiId
	data["DeviceTypes"] = deviceTypes
	data["ValidImageSuffixes"] = validImageSuffixes
	data["ScanDepth"] = 1
	data["FreeDiskSpace"] = freeDiskSpace

	c.HTML(http.StatusOK, "index.html", data)
}

// getBaseTemplateData returns common data for all templates
func (s *Server) getBaseTemplateData(c *gin.Context) gin.H {
	username := getUsername(c)

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown"
	}

	// Get server version from backend
	version := "Unknown"
	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ServerInfo())
	if err == nil && result.GetStatus() {
		serverInfo := result.GetServerInfo()
		if serverInfo != nil {
			version = fmt.Sprintf("%d.%d.%d",
				serverInfo.GetVersionInfo().GetMajorVersion(),
				serverInfo.GetVersionInfo().GetMinorVersion(),
				serverInfo.GetVersionInfo().GetPatchVersion())
		}
	}

	// Get theme from session (default to "modern")
	theme := "modern"
	session, err := s.getSession(c)
	if err == nil {
		if sessionTheme, ok := session.Values["theme"].(string); ok && sessionTheme != "" {
			theme = sessionTheme
		}
	}

	// Get flash messages from session
	flashMessage := ""
	errorMessage := ""
	if err == nil {
		flashMessage, errorMessage = GetFlashesForTemplate(session)
		// Save session to clear flash messages
		session.Save(c.Request, c.Writer)
	}

	return gin.H{
		"Username":     username,
		"Title":        "PiSCSI Control",
		"Hostname":     hostname,
		"Version":      version,
		"Theme":        theme,
		"FlashMessage": flashMessage,
		"ErrorMessage": errorMessage,
	}
}

// ResponseOptions configures the unified response handler
type ResponseOptions struct {
	Message     string
	Error       bool
	RedirectURL string
	Template    string
	Data        gin.H
	StatusCode  int
}

// respond provides unified response handling for both HTML (form posts) and JSON (API) requests
// For HTML: Sets flash message in session and redirects (POST-Redirect-GET pattern)
// For JSON: Returns JSON response with status and message
func (s *Server) respond(c *gin.Context, opts ResponseOptions) {
	// Default status code
	if opts.StatusCode == 0 {
		if opts.Error {
			opts.StatusCode = http.StatusBadRequest
		} else {
			opts.StatusCode = http.StatusOK
		}
	}

	// Default redirect URL
	if opts.RedirectURL == "" {
		opts.RedirectURL = "/"
	}

	// Check if client wants JSON response
	acceptHeader := c.GetHeader("Accept")
	if acceptHeader == "application/json" {
		// Return JSON response
		status := "success"
		if opts.Error {
			status = "error"
		}

		response := gin.H{
			"status":  status,
			"message": opts.Message,
		}

		// Add any additional data
		if opts.Data != nil {
			for key, value := range opts.Data {
				response[key] = value
			}
		}

		c.JSON(opts.StatusCode, response)
		return
	}

	// HTML response: Set flash message and redirect
	session, err := s.getSession(c)
	if err == nil {
		// Set flash message
		category := FlashSuccess
		if opts.Error {
			category = FlashError
		}
		if opts.Message != "" {
			SetFlash(session, opts.Message, category)
		}
		// Save session before any response is written
		if err := session.Save(c.Request, c.Writer); err != nil {
			s.logger.Error("Failed to save session", "error", err)
		}
	}

	// If template specified, render it instead of redirecting
	if opts.Template != "" {
		data := s.getBaseTemplateData(c)
		if opts.Data != nil {
			for key, value := range opts.Data {
				data[key] = value
			}
		}
		c.HTML(opts.StatusCode, opts.Template, data)
		return
	}

	// Redirect to specified URL (default: index)
	// Use manual header setting to ensure session cookie is written first
	c.Header("Location", opts.RedirectURL)
	c.Status(http.StatusSeeOther)
}

// getImageFiles returns a list of image files in the base directory
func (s *Server) getImageFiles() ([]map[string]interface{}, map[string][]map[string]interface{}) {
	files := []map[string]interface{}{}
	filesBySubdir := make(map[string][]map[string]interface{})

	if _, err := os.Stat(s.config.BaseDir); os.IsNotExist(err) {
		return files, filesBySubdir
	}

	// Walk the base directory
	filepath.Walk(s.config.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Check if it's an image file
		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		validExts := []string{"hda", "hds", "hdi", "hdf", "iso", "cdr", "img"}
		isValid := false
		for _, validExt := range validExts {
			if strings.EqualFold(ext, validExt) {
				isValid = true
				break
			}
		}

		if !isValid {
			return nil
		}

		sizeMB := info.Size() / (1024 * 1024)
		file := map[string]interface{}{
			"Name":            path,
			"SizeMB":          sizeMB,
			"InUse":           false,
			"DetectedType":    "",
			"DetectedTypeName": "",
		}

		files = append(files, file)

		// Group by subdirectory
		dir := filepath.Dir(path)
		if dir == "" {
			dir = s.config.BaseDir
		}
		filesBySubdir[dir] = append(filesBySubdir[dir], file)

		return nil
	})

	return files, filesBySubdir
}

// handleLoginPage serves the login form
func (s *Server) handleLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"Title": "Login - PiSCSI",
	})
}

// handleLogin processes login requests
func (s *Server) handleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"Error": "Username and password are required",
			"Title": "Login - PiSCSI",
		})
		return
	}

	// Authenticate using PAM (will be implemented later)
	// For now, use a simple check for development
	authenticated := s.authenticate(username, password)

	if !authenticated {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"Error": "Invalid username or password",
			"Title": "Login - PiSCSI",
		})
		return
	}

	// Create session
	session, err := s.sessionStore.Get(c.Request, sessionName)
	if err != nil {
		s.logger.Error("Failed to get session", "error", err)
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"Error": "Failed to create session",
			"Title": "Login - PiSCSI",
		})
		return
	}

	session.Values[sessionAuthKey] = true
	session.Values[sessionUserKey] = username

	if err := session.Save(c.Request, c.Writer); err != nil {
		s.logger.Error("Failed to save session", "error", err)
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"Error": "Failed to save session",
			"Title": "Login - PiSCSI",
		})
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// handleLogout logs the user out
func (s *Server) handleLogout(c *gin.Context) {
	session, err := s.sessionStore.Get(c.Request, sessionName)
	if err != nil {
		s.logger.Error("Failed to get session", "error", err)
	} else {
		// Clear session
		session.Values[sessionAuthKey] = false
		session.Values[sessionUserKey] = ""
		session.Options.MaxAge = -1

		if err := session.Save(c.Request, c.Writer); err != nil {
			s.logger.Error("Failed to save session", "error", err)
		}
	}

	c.Redirect(http.StatusFound, "/login")
}

// handleHealthcheck returns server health status
func (s *Server) handleHealthcheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

// handleEnv returns environment information
func (s *Server) handleEnv(c *gin.Context) {
	// Get server info from piscsi daemon
	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ServerInfo())

	if err != nil {
		s.logger.Error("Failed to get server info", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to communicate with piscsi daemon",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": result.Status,
		"data":   result,
	})
}

// handleAttach attaches a SCSI device
func (s *Server) handleAttach(c *gin.Context) {
	// Parse form data
	scsiID := c.PostForm("scsi_id")
	unit := c.DefaultPostForm("unit", "0")
	deviceType := c.PostForm("type")
	file := c.PostForm("file")

	if scsiID == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "SCSI ID is required",
		})
		return
	}

	// Parse IDs
	id, err := parseIntParam(scsiID)
	if err != nil || id < 0 || id > 7 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid SCSI ID (must be 0-7)",
		})
		return
	}

	lun, err := parseIntParam(unit)
	if err != nil || lun < 0 || lun > 31 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid LUN (must be 0-31)",
		})
		return
	}

	// Map device type string to protobuf enum
	pbType, err := parseDeviceType(deviceType)
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid device type",
		})
		return
	}

	// Build additional parameters
	params := make(map[string]string)
	if vendor := c.PostForm("vendor"); vendor != "" {
		params["vendor"] = vendor
	}
	if product := c.PostForm("product"); product != "" {
		params["product"] = product
	}
	if revision := c.PostForm("revision"); revision != "" {
		params["revision"] = revision
	}

	// Send attach command
	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(
		cmdBuilder.AttachDevice(id, lun, pbType, file, params),
	)

	if err != nil {
		s.logger.Error("Failed to attach device", "error", err)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to communicate with piscsi daemon: " + err.Error(),
		})
		return
	}

	// Check result status
	if !result.Status {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: result.Msg,
		})
		return
	}

	// Success message with device details
	message := fmt.Sprintf("Attached %s device to SCSI ID %s:%s", deviceType, scsiID, unit)
	if file != "" {
		message = fmt.Sprintf("Attached %s (%s) to SCSI ID %s:%s", deviceType, file, scsiID, unit)
	}

	s.respond(c, ResponseOptions{
		Message: message,
	})
}

// handleDetach detaches a SCSI device
func (s *Server) handleDetach(c *gin.Context) {
	// Parse form data
	scsiID := c.PostForm("scsi_id")
	unit := c.DefaultPostForm("unit", "0")

	if scsiID == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "SCSI ID is required",
		})
		return
	}

	id, err := parseIntParam(scsiID)
	if err != nil || id < 0 || id > 7 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid SCSI ID (must be 0-7)",
		})
		return
	}

	lun, err := parseIntParam(unit)
	if err != nil || lun < 0 || lun > 31 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid LUN (must be 0-31)",
		})
		return
	}

	// Send detach command
	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(
		cmdBuilder.DetachDevice(id, lun),
	)

	if err != nil {
		s.logger.Error("Failed to detach device", "error", err)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to communicate with piscsi daemon: " + err.Error(),
		})
		return
	}

	if !result.Status {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: result.Msg,
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Detached device from SCSI ID %s:%s", scsiID, unit),
	})
}

// handleDetachAll detaches all SCSI devices
func (s *Server) handleDetachAll(c *gin.Context) {
	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.DetachAll())

	if err != nil {
		s.logger.Error("Failed to detach all devices", "error", err)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to communicate with piscsi daemon: " + err.Error(),
		})
		return
	}

	if !result.Status {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: result.Msg,
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: "Detached all devices",
	})
}

// handleEject ejects removable media
func (s *Server) handleEject(c *gin.Context) {
	scsiID := c.PostForm("scsi_id")
	unit := c.DefaultPostForm("unit", "0")

	if scsiID == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "SCSI ID is required",
		})
		return
	}

	id, err := parseIntParam(scsiID)
	if err != nil || id < 0 || id > 7 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid SCSI ID (must be 0-7)",
		})
		return
	}

	lun, err := parseIntParam(unit)
	if err != nil || lun < 0 || lun > 31 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid LUN (must be 0-31)",
		})
		return
	}

	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(
		cmdBuilder.EjectDevice(id, lun),
	)

	if err != nil {
		s.logger.Error("Failed to eject device", "error", err)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to communicate with piscsi daemon: " + err.Error(),
		})
		return
	}

	if !result.Status {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: result.Msg,
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Ejected media from SCSI ID %s:%s", scsiID, unit),
	})
}

// handleFilesList lists available image files
func (s *Server) handleFilesList(c *gin.Context) {
	// Get images from piscsi daemon
	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(
		cmdBuilder.ListImages(s.config.BaseDir),
	)

	if err != nil {
		s.logger.Error("Failed to list images", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": false,
			"msg":    "Failed to communicate with piscsi daemon: " + err.Error(),
		})
		return
	}

	// Extract image file information from result
	files := make([]map[string]interface{}, 0)
	imageFilesInfo := result.GetImageFilesInfo()
	if imageFilesInfo != nil {
		for _, fileInfo := range imageFilesInfo.ImageFiles {
			files = append(files, map[string]interface{}{
				"name": fileInfo.Name,
				"size": fileInfo.Size,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"files":  files,
	})
}

// handleFilesUpload handles file uploads
func (s *Server) handleFilesUpload(c *gin.Context) {
	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "No file uploaded",
		})
		return
	}

	// Validate file size
	if file.Size > s.config.MaxFileSize {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("File too large (max %d bytes)", s.config.MaxFileSize),
		})
		return
	}

	// Sanitize filename
	filename := filepath.Base(file.Filename)
	if !isValidFilename(filename) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Determine destination directory
	destination := c.DefaultPostForm("destination", "images")
	var destPath string
	switch destination {
	case "images":
		destPath = s.config.BaseDir
	case "shared":
		destPath = s.config.SharedDir
	case "config":
		destPath = s.config.ConfigDir
	default:
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid destination",
		})
		return
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(destPath, 0755); err != nil {
		s.logger.Error("Failed to create directory", "error", err, "path", destPath)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to create destination directory",
		})
		return
	}

	// Save the file
	fullPath := filepath.Join(destPath, filename)
	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		s.logger.Error("Failed to save file", "error", err, "path", fullPath)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to save file: " + err.Error(),
		})
		return
	}

	s.logger.Info("File uploaded", "filename", filename, "size", file.Size, "destination", destPath)
	s.respond(c, ResponseOptions{
		Message: "File uploaded successfully",
		Data: gin.H{
			"filename": filename,
			"size":     file.Size,
		},
	})
}

// handleFilesDownload handles file downloads
func (s *Server) handleFilesDownload(c *gin.Context) {
	filename := c.Query("file")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "Filename is required",
		})
		return
	}

	// Sanitize filename to prevent path traversal
	filename = filepath.Base(filename)
	if !isValidFilename(filename) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "Invalid filename",
		})
		return
	}

	// Check which directory to download from
	source := c.DefaultQuery("source", "images")
	var sourcePath string
	switch source {
	case "images":
		sourcePath = s.config.BaseDir
	case "shared":
		sourcePath = s.config.SharedDir
	case "config":
		sourcePath = s.config.ConfigDir
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "Invalid source",
		})
		return
	}

	fullPath := filepath.Join(sourcePath, filename)

	// Security check: ensure the resolved path is within the allowed directory
	realPath, err := filepath.Abs(fullPath)
	if err != nil || !strings.HasPrefix(realPath, sourcePath) {
		c.JSON(http.StatusForbidden, gin.H{
			"status": false,
			"msg":    "Access denied",
		})
		return
	}

	// Check if file exists
	if _, err := os.Stat(realPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"status": false,
			"msg":    "File not found",
		})
		return
	}

	// Serve the file
	c.File(realPath)
}

// handleConfigList lists configuration files
func (s *Server) handleConfigList(c *gin.Context) {
	// List .properties files in config directory
	files, err := os.ReadDir(s.config.ConfigDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{
				"status": true,
				"files":  []string{},
			})
			return
		}
		s.logger.Error("Failed to read config directory", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": false,
			"msg":    "Failed to read configuration directory",
		})
		return
	}

	// Filter for .properties files
	configFiles := make([]map[string]interface{}, 0)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasSuffix(file.Name(), ".properties") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			configFiles = append(configFiles, map[string]interface{}{
				"name":    file.Name(),
				"size":    info.Size(),
				"modTime": info.ModTime().Unix(),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"files":  configFiles,
	})
}

// handleConfigSave saves current configuration
func (s *Server) handleConfigSave(c *gin.Context) {
	filename := c.PostForm("filename")
	if filename == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Filename is required",
		})
		return
	}

	// Sanitize filename
	filename = filepath.Base(filename)
	if !strings.HasSuffix(filename, ".properties") {
		filename += ".properties"
	}

	if !isValidFilename(filename) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Get current device list from daemon
	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ListDevices())

	if err != nil {
		s.logger.Error("Failed to get device list for config save", "error", err)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to communicate with piscsi daemon: " + err.Error(),
		})
		return
	}

	// Build configuration file content
	configContent := buildConfigContent(result)

	// Ensure config directory exists
	if err := os.MkdirAll(s.config.ConfigDir, 0755); err != nil {
		s.logger.Error("Failed to create config directory", "error", err)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to create configuration directory",
		})
		return
	}

	// Write configuration file
	fullPath := filepath.Join(s.config.ConfigDir, filename)
	if err := os.WriteFile(fullPath, []byte(configContent), 0644); err != nil {
		s.logger.Error("Failed to write config file", "error", err, "path", fullPath)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to write configuration file",
		})
		return
	}

	s.logger.Info("Configuration saved", "filename", filename)
	s.respond(c, ResponseOptions{
		Message: "Configuration saved successfully",
		Data: gin.H{
			"filename": filename,
		},
	})
}

// handleConfigLoad loads a configuration
func (s *Server) handleConfigLoad(c *gin.Context) {
	filename := c.PostForm("filename")
	if filename == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Filename is required",
		})
		return
	}

	// Sanitize filename
	filename = filepath.Base(filename)
	if !isValidFilename(filename) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	fullPath := filepath.Join(s.config.ConfigDir, filename)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Configuration file not found",
		})
		return
	}

	// Read configuration file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		s.logger.Error("Failed to read config file", "error", err, "path", fullPath)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to read configuration file",
		})
		return
	}

	// Parse and apply configuration
	// For now, we'll just detach all and log the config
	// A full implementation would parse the .properties file and attach devices accordingly
	cmdBuilder := s.getCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.DetachAll())

	if err != nil {
		s.logger.Error("Failed to detach devices", "error", err)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to communicate with piscsi daemon: " + err.Error(),
		})
		return
	}

	if !result.Status {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to detach devices: " + result.Msg,
		})
		return
	}

	// TODO: Parse configuration file and attach devices
	// This is a placeholder - full implementation would parse the .properties format
	s.logger.Info("Configuration loaded", "filename", filename, "content_size", len(content))

	s.respond(c, ResponseOptions{
		Message: "Configuration loaded (detached all devices, parsing not yet fully implemented)",
	})
}

// authenticate checks username/password using PAM
func (s *Server) authenticate(username, password string) bool {
	err := s.authenticator.AuthenticateAndAuthorize(username, password)
	if err != nil {
		s.logger.Warn("Authentication failed", "username", username, "error", err)
		return false
	}
	return true
}

// getCommandBuilder creates a new command builder
func (s *Server) getCommandBuilder() *piscsi.CommandBuilder {
	return piscsi.NewCommandBuilder()
}

// Helper functions

// parseIntParam parses a string parameter to int32
func parseIntParam(s string) (int32, error) {
	var val int
	_, err := fmt.Sscanf(s, "%d", &val)
	if err != nil {
		return 0, err
	}
	return int32(val), nil
}

// parseDeviceType converts a string device type to protobuf enum
func parseDeviceType(deviceType string) (pb.PbDeviceType, error) {
	switch deviceType {
	case "SCHD", "schd":
		return pb.PbDeviceType_SCHD, nil
	case "SCRM", "scrm":
		return pb.PbDeviceType_SCRM, nil
	case "SCCD", "sccd":
		return pb.PbDeviceType_SCCD, nil
	case "SCMO", "scmo":
		return pb.PbDeviceType_SCMO, nil
	case "SCBR", "scbr":
		return pb.PbDeviceType_SCBR, nil
	case "SCDP", "scdp":
		return pb.PbDeviceType_SCDP, nil
	default:
		return pb.PbDeviceType_UNDEFINED, fmt.Errorf("unknown device type: %s", deviceType)
	}
}

// isValidFilename checks if a filename is safe (no path traversal, no special chars)
func isValidFilename(filename string) bool {
	// Check for empty filename
	if filename == "" {
		return false
	}

	// Check for path traversal attempts
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return false
	}

	// Check for hidden files (starting with .)
	if strings.HasPrefix(filename, ".") {
		return false
	}

	// Check filename length (reasonable limit)
	if len(filename) > 255 {
		return false
	}

	return true
}

// buildConfigContent creates a .properties file content from current device list
func buildConfigContent(result *pb.PbResult) string {
	var builder strings.Builder

	builder.WriteString("# PiSCSI Configuration\n")
	builder.WriteString(fmt.Sprintf("# Generated at: %s\n\n", filepath.Base(os.Args[0])))

	devicesInfo := result.GetDevicesInfo()
	if devicesInfo == nil || len(devicesInfo.Devices) == 0 {
		builder.WriteString("# No devices attached\n")
		return builder.String()
	}

	for _, device := range devicesInfo.Devices {
		// Format: device.ID.UNIT=TYPE:FILE
		builder.WriteString(fmt.Sprintf("device.%d.%d=%s",
			device.Id, device.Unit, device.Type.String()))

		// Add file parameter if present
		if device.GetParams() != nil {
			if file, ok := device.GetParams()["file"]; ok {
				builder.WriteString(":" + file)
			}
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

// handleScsiInfo retrieves detailed info for all attached devices
func (s *Server) handleScsiInfo(c *gin.Context) {
	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ListDevices())
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to get device info: %v", err),
		})
		return
	}

	devicesInfo := result.GetDevicesInfo()
	if devicesInfo == nil || len(devicesInfo.GetDevices()) == 0 {
		s.respond(c, ResponseOptions{
			Message: "No devices attached",
			Data:    gin.H{"devices": []map[string]interface{}{}},
		})
		return
	}

	// Format device information
	devices := make([]map[string]interface{}, 0)
	for _, device := range devicesInfo.GetDevices() {
		deviceInfo := map[string]interface{}{
			"id":          device.GetId(),
			"unit":        device.GetUnit(),
			"type":        device.GetType().String(),
			"vendor":      device.GetVendor(),
			"product":     device.GetProduct(),
			"revision":    device.GetRevision(),
			"block_size":  device.GetBlockSize(),
			"block_count": device.GetBlockCount(),
		}

		// Add file if present
		if params := device.GetParams(); params != nil {
			if file, ok := params["file"]; ok {
				deviceInfo["file"] = file
			}
		}

		// Add status
		if status := device.GetStatus(); status != nil {
			deviceInfo["status"] = status.String()
		}

		devices = append(devices, deviceInfo)
	}

	s.respond(c, ResponseOptions{
		Message: "Retrieved device information",
		Data:    gin.H{"devices": devices},
	})
}

// handleScsiReserve reserves a SCSI ID
func (s *Server) handleScsiReserve(c *gin.Context) {
	scsiIDStr := c.PostForm("scsi_id")
	memo := c.PostForm("memo")

	// Parse and validate SCSI ID
	scsiID, err := parseIntParam(scsiIDStr)
	if err != nil || scsiID < 0 || scsiID > 7 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid SCSI ID (must be 0-7)",
		})
		return
	}

	// Get current reserved IDs
	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ServerInfo())
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to get server info: %v", err),
		})
		return
	}

	// Get reserved IDs from server info
	reservedIDsInfo := result.GetReservedIdsInfo()
	var currentIDs []int32
	if reservedIDsInfo != nil {
		currentIDs = reservedIDsInfo.GetIds()
	}

	// Check if ID is already reserved
	for _, id := range currentIDs {
		if id == scsiID {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("SCSI ID %d is already reserved", scsiID),
			})
			return
		}
	}

	// Add new ID to reserved list
	newIDs := append(currentIDs, scsiID)

	// Send reserve command
	result, err = s.piscsiClient.SendCommand(cmdBuilder.ReserveIDs(newIDs))
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to reserve SCSI ID: %v", err),
		})
		return
	}

	// Check if command succeeded
	if !result.GetStatus() {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to reserve SCSI ID: %s", result.GetMsg()),
		})
		return
	}

	// Note: memo storage is handled client-side in Python version
	// We return it in the response for the client to store
	data := gin.H{}
	if memo != "" {
		data["memo"] = memo
	}
	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Reserved SCSI ID %d", scsiID),
		Data:    data,
	})
}

// handleScsiRelease releases a reserved SCSI ID
func (s *Server) handleScsiRelease(c *gin.Context) {
	scsiIDStr := c.PostForm("scsi_id")

	// Parse and validate SCSI ID
	scsiID, err := parseIntParam(scsiIDStr)
	if err != nil || scsiID < 0 || scsiID > 7 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid SCSI ID (must be 0-7)",
		})
		return
	}

	// Get current reserved IDs
	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ServerInfo())
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to get server info: %v", err),
		})
		return
	}

	// Get reserved IDs from server info
	reservedIDsInfo := result.GetReservedIdsInfo()
	var currentIDs []int32
	if reservedIDsInfo != nil {
		currentIDs = reservedIDsInfo.GetIds()
	}

	// Remove the ID from reserved list
	newIDs := make([]int32, 0)
	found := false
	for _, id := range currentIDs {
		if id != scsiID {
			newIDs = append(newIDs, id)
		} else {
			found = true
		}
	}

	if !found {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("SCSI ID %d is not currently reserved", scsiID),
		})
		return
	}

	// Send reserve command with updated list
	result, err = s.piscsiClient.SendCommand(cmdBuilder.ReserveIDs(newIDs))
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to release SCSI ID: %v", err),
		})
		return
	}

	// Check if command succeeded
	if !result.GetStatus() {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to release SCSI ID: %s", result.GetMsg()),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Released the reservation for SCSI ID %d", scsiID),
	})
}

// handleFilesDelete deletes a file from the images directory
func (s *Server) handleFilesDelete(c *gin.Context) {
	fileName := c.PostForm("file_name")
	if fileName == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "File name is required",
		})
		return
	}

	// Validate path safety
	if !isValidFilename(fileName) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Construct full path
	fullPath := filepath.Join(s.config.BaseDir, fileName)

	// Verify path is within base directory
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(s.config.BaseDir)) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid file path",
		})
		return
	}

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		s.respond(c, ResponseOptions{
			Error:      true,
			Message:    fmt.Sprintf("File not found: %s", fileName),
			StatusCode: http.StatusNotFound,
		})
		return
	}

	// Delete the file
	if err := os.Remove(fullPath); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to delete file: %v", err),
		})
		return
	}

	// Delete properties file if it exists
	propFileName := fileName + ".properties"
	propPath := filepath.Join(s.config.ConfigDir, propFileName)
	propDeleted := false
	if _, err := os.Stat(propPath); err == nil {
		if err := os.Remove(propPath); err == nil {
			propDeleted = true
		}
	}

	message := fmt.Sprintf("Image file deleted: %s", fileName)
	if propDeleted {
		message = fmt.Sprintf("Image file with properties deleted: %s", fileName)
	}

	s.respond(c, ResponseOptions{
		Message: message,
	})
}

// handleFilesRename renames a file in the images directory
func (s *Server) handleFilesRename(c *gin.Context) {
	fileName := c.PostForm("file_name")
	newFileName := c.PostForm("new_file_name")

	if fileName == "" || newFileName == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "File name and new file name are required",
		})
		return
	}

	// Validate both filenames
	if !isValidFilename(fileName) || !isValidFilename(newFileName) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Construct full paths
	oldPath := filepath.Join(s.config.BaseDir, fileName)
	newPath := filepath.Join(s.config.BaseDir, newFileName)

	// Verify paths are within base directory
	cleanOldPath := filepath.Clean(oldPath)
	cleanNewPath := filepath.Clean(newPath)
	baseDir := filepath.Clean(s.config.BaseDir)
	if !strings.HasPrefix(cleanOldPath, baseDir) || !strings.HasPrefix(cleanNewPath, baseDir) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid file path",
		})
		return
	}

	// Check if source file exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		s.respond(c, ResponseOptions{
			Error:      true,
			Message:    fmt.Sprintf("File not found: %s", fileName),
			StatusCode: http.StatusNotFound,
		})
		return
	}

	// Check if destination already exists
	if _, err := os.Stat(newPath); err == nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("File already exists: %s", newFileName),
		})
		return
	}

	// Rename the file
	if err := os.Rename(oldPath, newPath); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to rename file: %v", err),
		})
		return
	}

	// Rename properties file if it exists
	oldPropPath := filepath.Join(s.config.ConfigDir, fileName+".properties")
	newPropPath := filepath.Join(s.config.ConfigDir, newFileName+".properties")
	propRenamed := false
	if _, err := os.Stat(oldPropPath); err == nil {
		if err := os.Rename(oldPropPath, newPropPath); err == nil {
			propRenamed = true
		}
	}

	message := fmt.Sprintf("Image file renamed to: %s", newFileName)
	if propRenamed {
		message = fmt.Sprintf("Image file with properties renamed to: %s", newFileName)
	}

	s.respond(c, ResponseOptions{
		Message: message,
	})
}

// handleFilesCopy creates a copy of a file in the images directory
func (s *Server) handleFilesCopy(c *gin.Context) {
	fileName := c.PostForm("file_name")
	copyFileName := c.PostForm("copy_file_name")

	if fileName == "" || copyFileName == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "File name and copy file name are required",
		})
		return
	}

	// Validate both filenames
	if !isValidFilename(fileName) || !isValidFilename(copyFileName) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Construct full paths
	srcPath := filepath.Join(s.config.BaseDir, fileName)
	dstPath := filepath.Join(s.config.BaseDir, copyFileName)

	// Verify paths are within base directory
	cleanSrcPath := filepath.Clean(srcPath)
	cleanDstPath := filepath.Clean(dstPath)
	baseDir := filepath.Clean(s.config.BaseDir)
	if !strings.HasPrefix(cleanSrcPath, baseDir) || !strings.HasPrefix(cleanDstPath, baseDir) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid file path",
		})
		return
	}

	// Check if source file exists
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		s.respond(c, ResponseOptions{
			Error:      true,
			Message:    fmt.Sprintf("File not found: %s", fileName),
			StatusCode: http.StatusNotFound,
		})
		return
	}

	// Check if destination already exists
	if _, err := os.Stat(dstPath); err == nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("File already exists: %s", copyFileName),
		})
		return
	}

	// Copy the file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to open source file: %v", err),
		})
		return
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to create destination file: %v", err),
		})
		return
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dstPath) // Clean up on failure
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to copy file: %v", err),
		})
		return
	}

	// Copy properties file if it exists
	srcPropPath := filepath.Join(s.config.ConfigDir, fileName+".properties")
	dstPropPath := filepath.Join(s.config.ConfigDir, copyFileName+".properties")
	propCopied := false
	if _, err := os.Stat(srcPropPath); err == nil {
		if srcPropFile, err := os.Open(srcPropPath); err == nil {
			defer srcPropFile.Close()
			if dstPropFile, err := os.Create(dstPropPath); err == nil {
				defer dstPropFile.Close()
				if _, err := io.Copy(dstPropFile, srcPropFile); err == nil {
					propCopied = true
				}
			}
		}
	}

	message := fmt.Sprintf("Copy of image file saved as: %s", copyFileName)
	if propCopied {
		message = fmt.Sprintf("Copy of image file with properties saved as: %s", copyFileName)
	}

	s.respond(c, ResponseOptions{
		Message: message,
	})
}

// handleFilesDownloadConfig downloads a configuration file
func (s *Server) handleFilesDownloadConfig(c *gin.Context) {
	fileName := c.PostForm("file")
	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "File name is required",
		})
		return
	}

	// Validate filename
	if !isValidFilename(fileName) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "Invalid filename",
		})
		return
	}

	// Construct full path
	fullPath := filepath.Join(s.config.ConfigDir, fileName)

	// Verify path is within config directory
	cleanPath := filepath.Clean(fullPath)
	configDir := filepath.Clean(s.config.ConfigDir)
	if !strings.HasPrefix(cleanPath, configDir) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "Invalid file path",
		})
		return
	}

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"status": false,
			"msg":    fmt.Sprintf("File not found: %s", fileName),
		})
		return
	}

	// Serve the file for download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.File(fullPath)
}

// handleConfigAction performs an action on a configuration file (load, delete, or send)
func (s *Server) handleConfigAction(c *gin.Context) {
	fileName := c.PostForm("name")
	if fileName == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "File name is required",
		})
		return
	}

	// Validate filename
	if !isValidFilename(fileName) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Construct full path
	fullPath := filepath.Join(s.config.ConfigDir, fileName)

	// Verify path is within config directory
	cleanPath := filepath.Clean(fullPath)
	configDir := filepath.Clean(s.config.ConfigDir)
	if !strings.HasPrefix(cleanPath, configDir) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid file path",
		})
		return
	}

	// Check which action to perform
	if c.PostForm("load") != "" {
		// Load configuration - reuse existing handleConfigLoad logic
		// Read configuration file
		configData, err := os.ReadFile(fullPath)
		if err != nil {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("Failed to read configuration file: %v", err),
			})
			return
		}

		// Parse and apply configuration (simplified - detach all devices first)
		cmdBuilder := piscsi.NewCommandBuilder()
		result, err := s.piscsiClient.SendCommand(cmdBuilder.DetachAll())
		if err != nil {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("Failed to detach devices: %v", err),
			})
			return
		}

		if !result.GetStatus() {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("Failed to detach devices: %s", result.GetMsg()),
			})
			return
		}

		s.respond(c, ResponseOptions{
			Message: fmt.Sprintf("Configuration loaded from %s", fileName),
			Data: gin.H{
				"config": string(configData),
			},
		})
		return
	}

	if c.PostForm("delete") != "" {
		// Delete configuration file
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("File not found: %s", fileName),
			})
			return
		}

		if err := os.Remove(fullPath); err != nil {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("Failed to delete file: %v", err),
			})
			return
		}

		s.respond(c, ResponseOptions{
			Message: fmt.Sprintf("Configuration file deleted: %s", fileName),
		})
		return
	}

	if c.PostForm("send") != "" {
		// Download configuration file
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("File not found: %s", fileName),
			})
			return
		}

		c.Header("Content-Description", "File Transfer")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
		c.File(fullPath)
		return
	}

	// No recognized action
	s.respond(c, ResponseOptions{
		Error:   true,
		Message: "No known operation in request. Expected one of: load, delete, send",
	})
}

// handleLogsLevel sets the PiSCSI backend log level
func (s *Server) handleLogsLevel(c *gin.Context) {
	level := c.DefaultPostForm("level", "info")

	// Validate log level
	validLevels := map[string]bool{
		"trace": true,
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	if !validLevels[level] {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Invalid log level: %s (must be trace, debug, info, warn, or error)", level),
		})
		return
	}

	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.SetLogLevel(level))
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to set log level: %v", err),
		})
		return
	}

	if !result.GetStatus() {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to set log level: %s", result.GetMsg()),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Log level set to %s", level),
	})
}

// handleLogsShow displays system logs
func (s *Server) handleLogsShow(c *gin.Context) {
	lines := c.DefaultPostForm("lines", "100")
	scope := c.PostForm("scope")

	// Build journalctl command
	args := []string{}
	if lines != "" {
		args = append(args, "-n", lines)
	}
	if scope != "" {
		args = append(args, "-u", scope)
	}

	// Execute journalctl command
	cmd := exec.Command("journalctl", args...)
	output, err := cmd.CombinedOutput()

	logs := string(output)
	if err != nil {
		logs = fmt.Sprintf("An error occurred when fetching logs: %v", err)
	}

	// Prepare scope display text
	scopeDisplay := "All logs"
	if scope != "" {
		scopeDisplay = scope
	}

	// Render the logs template
	data := s.getBaseTemplateData(c)
	data["Scope"] = scopeDisplay
	data["Lines"] = lines
	data["Logs"] = logs
	data["Title"] = "PiSCSI System Logs"

	c.HTML(http.StatusOK, "logs.html", data)
}

// handleSysRename changes the system hostname
func (s *Server) handleSysRename(c *gin.Context) {
	systemName := c.PostForm("system_name")

	const maxLength = 120
	if len(systemName) > maxLength {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("System name too long (max %d characters)", maxLength),
		})
		return
	}

	// Execute hostnamectl command
	cmd := exec.Command("sudo", "hostnamectl", "set-hostname", "--pretty", systemName)
	if err := cmd.Run(); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Failed to change system name.",
		})
		return
	}

	message := "System name reset to default."
	if systemName != "" {
		message = fmt.Sprintf("System name changed to '%s'.", systemName)
	}

	s.respond(c, ResponseOptions{
		Message: message,
	})
}

// handleSysReboot restarts the system
func (s *Server) handleSysReboot(c *gin.Context) {
	// Execute reboot command
	cmd := exec.Command("sudo", "reboot")
	output, err := cmd.CombinedOutput()

	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: string(output),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: "System reboot initiated",
	})
}

// handleSysShutdown shuts down the system
func (s *Server) handleSysShutdown(c *gin.Context) {
	// Execute shutdown command
	cmd := exec.Command("sudo", "shutdown", "-h", "now")
	output, err := cmd.CombinedOutput()

	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: string(output),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: "System shutdown initiated",
	})
}

// handleFilesCreate creates a blank disk image file
func (s *Server) handleFilesCreate(c *gin.Context) {
	fileName := c.PostForm("file_name")
	sizeStr := c.PostForm("size") // Size in MB
	fileType := c.DefaultPostForm("type", "hda")

	if fileName == "" || sizeStr == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "File name and size are required",
		})
		return
	}

	// Validate filename
	if !isValidFilename(fileName) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Parse size (in MB)
	var sizeMB int64
	if _, err := fmt.Sscanf(sizeStr, "%d", &sizeMB); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid size",
		})
		return
	}

	// Construct full filename
	fullFileName := fmt.Sprintf("%s.%s", fileName, fileType)
	fullPath := filepath.Join(s.config.BaseDir, fullFileName)

	// Verify path is within base directory
	cleanPath := filepath.Clean(fullPath)
	baseDir := filepath.Clean(s.config.BaseDir)
	if !strings.HasPrefix(cleanPath, baseDir) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid file path",
		})
		return
	}

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("File already exists: %s", fullFileName),
		})
		return
	}

	// Create blank disk image using dd
	sizeBytes := sizeMB * 1024 * 1024
	cmd := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s", fullPath),
		"bs=1M", fmt.Sprintf("count=%d", sizeMB), "status=none")

	if err := cmd.Run(); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to create image file: %v", err),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Image file created: %s", fullFileName),
		Data: gin.H{
			"image": fullFileName,
			"size":  sizeBytes,
		},
	})
}

// handleDriveCreate creates an image and properties file pair
func (s *Server) handleDriveCreate(c *gin.Context) {
	fileName := c.PostForm("file_name")
	driveName := c.PostForm("drive_name")

	if fileName == "" || driveName == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "File name and drive name are required",
		})
		return
	}

	// Check if drive properties are loaded
	if s.driveProps == nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Drive properties database not available",
		})
		return
	}

	// Get drive properties by name
	props, err := s.driveProps.GetByName(driveName)
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("No properties data for drive %s", driveName),
		})
		return
	}

	// Check that we have a valid file type and size
	if props.FileType == nil || *props.FileType == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Drive %s has no file type defined", driveName),
		})
		return
	}

	if props.Size == nil || *props.Size == 0 {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Drive %s has no size defined", driveName),
		})
		return
	}

	// Create the full filename with extension
	fullFileName := fmt.Sprintf("%s.%s", fileName, *props.FileType)
	fullPath := filepath.Join(s.config.BaseDir, fullFileName)

	// Create the empty image file using dd (same as handleFilesCreate)
	sizeMB := *props.Size / (1024 * 1024)
	cmd := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s", fullPath),
		"bs=1M", fmt.Sprintf("count=%d", sizeMB), "status=none")

	if err := cmd.Run(); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to create image file: %v", err),
		})
		return
	}

	// Create properties file in the same directory as the image
	propFileName := fmt.Sprintf("%s.properties", fullFileName)
	propFilePath := filepath.Join(s.config.BaseDir, propFileName)

	// Build properties JSON
	propData := map[string]interface{}{
		"vendor":     props.Vendor,
		"product":    props.Product,
		"revision":   props.Revision,
		"block_size": props.BlockSize,
		"size":       props.Size,
		"file_type":  props.FileType,
	}

	propJSON, err := json.MarshalIndent(propData, "", "    ")
	if err != nil {
		// Clean up the image file if we can't create properties
		os.Remove(fullPath)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to serialize properties: %v", err),
		})
		return
	}

	if err := os.WriteFile(propFilePath, propJSON, 0644); err != nil {
		// Clean up the image file if we can't create properties
		os.Remove(fullPath)
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to write properties file: %v", err),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Image file with properties created: %s", fullFileName),
	})
}

// handleDriveCdrom creates a properties file for a CD-ROM image
func (s *Server) handleDriveCdrom(c *gin.Context) {
	fileName := c.PostForm("file_name")
	driveName := c.PostForm("drive_name")

	if fileName == "" || driveName == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "File name and drive name are required",
		})
		return
	}

	// Check if drive properties are loaded
	if s.driveProps == nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Drive properties database not available",
		})
		return
	}

	// Get drive properties by name
	props, err := s.driveProps.GetByName(driveName)
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("No properties data for drive %s", driveName),
		})
		return
	}

	// Create properties file for the image (without creating the image itself)
	propFileName := fmt.Sprintf("%s.properties", fileName)
	propFilePath := filepath.Join(s.config.BaseDir, propFileName)

	// Build properties JSON
	propData := map[string]interface{}{
		"vendor":     props.Vendor,
		"product":    props.Product,
		"revision":   props.Revision,
		"block_size": props.BlockSize,
		"size":       props.Size,
	}

	propJSON, err := json.MarshalIndent(propData, "", "    ")
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to serialize properties: %v", err),
		})
		return
	}

	if err := os.WriteFile(propFilePath, propJSON, 0644); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to write properties file: %v", err),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Properties file created for CD-ROM image: %s", propFileName),
	})
}

// Page Routes (simplified - return JSON for now, templates in Phase 3)

// handleDriveList shows drive creation page
func (s *Server) handleDriveList(c *gin.Context) {
	// Get base template data
	data := s.getBaseTemplateData(c)
	data["Title"] = "PiSCSI - Create Drive"

	// Get list of available images
	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ListImages(s.config.BaseDir))
	if err != nil {
		data["ErrorMessage"] = fmt.Sprintf("Failed to list images: %v", err)
		c.HTML(http.StatusOK, "drives.html", data)
		return
	}

	// Get drive properties and categorize by type
	var hardDrives []map[string]interface{}
	var cdromDrives []map[string]interface{}
	var removableDrives []map[string]interface{}

	if s.driveProps != nil {
		allDrives := s.driveProps.GetAllDrives()
		for _, drive := range allDrives {
			prop := map[string]interface{}{
				"Name":        drive.Name,
				"Description": drive.Description,
				"URL":         drive.URL,
			}

			// Create secure filename (replace spaces with underscores)
			secureName := strings.ReplaceAll(drive.Name, " ", "_")
			prop["SecureName"] = secureName

			// Add file type if present
			if drive.FileType != nil {
				prop["FileType"] = *drive.FileType
			}

			// Add size in MB if defined
			if drive.Size != nil && *drive.Size > 0 {
				prop["SizeMB"] = *drive.Size / (1024 * 1024)
			}

			// Categorize by device type
			switch drive.DeviceType {
			case "SCHD":
				hardDrives = append(hardDrives, prop)
			case "SCCD":
				cdromDrives = append(cdromDrives, prop)
			case "SCRM":
				removableDrives = append(removableDrives, prop)
			}
		}
	}

	data["HardDrives"] = hardDrives
	data["CDRomDrives"] = cdromDrives
	data["RemovableDrives"] = removableDrives

	// Get list of existing CD-ROM image files
	imageFilesInfo := result.GetImageFilesInfo()
	cdImageFiles := make([]string, 0)
	if imageFilesInfo != nil {
		for _, imageFile := range imageFilesInfo.GetImageFiles() {
			name := imageFile.GetName()
			// Filter for CD-ROM images (iso, cue, cdr, toast)
			lowerName := strings.ToLower(name)
			if strings.HasSuffix(lowerName, ".iso") ||
				strings.HasSuffix(lowerName, ".cue") ||
				strings.HasSuffix(lowerName, ".cdr") ||
				strings.HasSuffix(lowerName, ".toast") {
				cdImageFiles = append(cdImageFiles, name)
			}
		}
	}
	data["CDImageFiles"] = cdImageFiles

	c.HTML(http.StatusOK, "drives.html", data)
}

// handleSysAdmin shows system administration page
func (s *Server) handleSysAdmin(c *gin.Context) {
	// Get base template data
	data := s.getBaseTemplateData(c)
	data["Title"] = "PiSCSI - Settings"

	// Get server info for log levels
	cmdBuilder := piscsi.NewCommandBuilder()
	result, err := s.piscsiClient.SendCommand(cmdBuilder.ServerInfo())

	logLevels := []string{"trace", "debug", "info", "warn", "error"}
	currentLogLevel := "info"

	if err == nil && result.GetStatus() {
		serverInfo := result.GetServerInfo()
		if serverInfo != nil && serverInfo.GetLogLevelInfo() != nil {
			currentLogLevel = serverInfo.GetLogLevelInfo().GetCurrentLogLevel()
		}
	}

	data["LogLevels"] = logLevels
	data["CurrentLogLevel"] = currentLogLevel

	// Get system configuration
	data["SystemName"] = data["Hostname"]

	// Theme information
	themes := []string{"modern", "classic"}
	data["Themes"] = themes
	data["CurrentTheme"] = data["Theme"]

	// Language/locale information (stub for now)
	type Locale struct {
		Language    string
		DisplayName string
	}
	locales := []Locale{
		{Language: "en", DisplayName: "English"},
		{Language: "de", DisplayName: "Deutsch"},
		{Language: "sv", DisplayName: "Svenska"},
		{Language: "fr", DisplayName: "Français"},
		{Language: "es", DisplayName: "Español"},
		{Language: "zh", DisplayName: "中文"},
	}
	data["Locales"] = locales
	data["CurrentLocale"] = "en" // Default to English for now

	// Service status checks (stub - these would check actual services)
	data["NetatalkConfigured"] = false
	data["SambaConfigured"] = false
	data["FtpConfigured"] = false
	data["MacproxyConfigured"] = false
	data["WebminConfigured"] = false

	// Get IP address
	data["IPAddress"] = c.ClientIP()

	c.HTML(http.StatusOK, "admin.html", data)
}

// handleUploadPage shows file upload page
func (s *Server) handleUploadPage(c *gin.Context) {
	// Get base template data
	data := s.getBaseTemplateData(c)
	data["Title"] = "PiSCSI - Upload"

	maxFileSizeMB := s.config.MaxFileSize / 1024 / 1024
	data["MaxFileSize"] = maxFileSizeMB
	data["ImageDir"] = s.config.BaseDir
	data["ConfigDir"] = s.config.ConfigDir
	data["ImageRootDir"] = s.config.BaseDir

	// Get subdirectories for upload destinations
	imagesSubdirs := []string{}
	filepath.Walk(s.config.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && path != s.config.BaseDir {
			relPath, _ := filepath.Rel(s.config.BaseDir, path)
			// Skip hidden directories
			if !strings.HasPrefix(relPath, ".") {
				imagesSubdirs = append(imagesSubdirs, relPath)
			}
		}
		return nil
	})
	data["ImagesSubdirs"] = imagesSubdirs

	// Check if shared directory exists
	sharedDir := filepath.Join(filepath.Dir(s.config.BaseDir), "shared_files")
	fileServerDirExists := false
	if _, err := os.Stat(sharedDir); err == nil {
		fileServerDirExists = true
	}
	data["FileServerDirExists"] = fileServerDirExists
	data["SharedDir"] = sharedDir
	data["SharedRootDir"] = sharedDir

	sharedSubdirs := []string{}
	if fileServerDirExists {
		filepath.Walk(sharedDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() && path != sharedDir {
				relPath, _ := filepath.Rel(sharedDir, path)
				if !strings.HasPrefix(relPath, ".") {
					sharedSubdirs = append(sharedSubdirs, relPath)
				}
			}
			return nil
		})
	}
	data["SharedSubdirs"] = sharedSubdirs

	c.HTML(http.StatusOK, "upload.html", data)
}

// handleFilesDiskinfo displays disk image information
func (s *Server) handleFilesDiskinfo(c *gin.Context) {
	fileName := c.PostForm("file_name")
	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "File name is required",
		})
		return
	}

	// Validate filename
	if !isValidFilename(fileName) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "Invalid filename",
		})
		return
	}

	// Construct full path
	fullPath := filepath.Join(s.config.BaseDir, fileName)

	// Verify path is within base directory
	cleanPath := filepath.Clean(fullPath)
	baseDir := filepath.Clean(s.config.BaseDir)
	if !strings.HasPrefix(cleanPath, baseDir) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"msg":    "Invalid file path",
		})
		return
	}

	// Check if file exists
	_, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		data := s.getBaseTemplateData(c)
		data["ErrorMessage"] = fmt.Sprintf("File not found: %s", fileName)
		data["Title"] = "Error"
		c.HTML(http.StatusNotFound, "diskinfo.html", data)
		return
	}

	// Get disk info using 'file' command
	cmd := exec.Command("file", "-b", fullPath)
	output, err := cmd.CombinedOutput()
	diskInfo := string(output)
	if err != nil {
		diskInfo = "Unable to determine file type"
	}

	// Get base template data
	data := s.getBaseTemplateData(c)
	data["FileName"] = fileName
	data["DiskInfo"] = diskInfo
	data["Title"] = "PiSCSI Image Info"

	c.HTML(http.StatusOK, "diskinfo.html", data)
}

// handleSysManpage displays manual pages
func (s *Server) handleSysManpage(c *gin.Context) {
	app := c.Query("app")
	allowlist := map[string]bool{
		"piscsi":   true,
		"scsictl":  true,
		"scsidump": true,
		"scsimon":  true,
	}

	if !allowlist[app] {
		data := s.getBaseTemplateData(c)
		data["ErrorMessage"] = fmt.Sprintf("%s is not a recognized PiSCSI app", app)
		data["Title"] = "Error"
		c.HTML(http.StatusBadRequest, "manpage.html", data)
		return
	}

	// Try to read man page and convert to HTML
	// Man pages should be at ../../../doc/{app}.1 relative to the binary
	// For now, we'll look in a few common locations
	manpagePaths := []string{
		fmt.Sprintf("doc/%s.1", app),
		fmt.Sprintf("../doc/%s.1", app),
		fmt.Sprintf("../../doc/%s.1", app),
		fmt.Sprintf("../../../doc/%s.1", app),
		fmt.Sprintf("/usr/share/man/man1/%s.1", app),
		fmt.Sprintf("/usr/local/share/man/man1/%s.1", app),
	}

	var manpageHTML string
	found := false

	for _, manPath := range manpagePaths {
		if _, err := os.Stat(manPath); err == nil {
			// Convert man page to HTML using man -Thtml
			cmd := exec.Command("man", "-Thtml", "-l", manPath)
			output, err := cmd.CombinedOutput()
			if err == nil {
				htmlContent := string(output)

				// Strip HTML wrapper tags (keep only body content)
				htmlToStrip := []string{
					"Content-type: text/html",
					"<!DOCTYPE",
					"<HTML>",
					"</HTML>",
					"<HEAD>",
					"</HEAD>",
					"<BODY>",
					"</BODY>",
					"<H1>",
					"</H1>",
				}

				for _, tag := range htmlToStrip {
					htmlContent = strings.ReplaceAll(htmlContent, tag, "")
				}

				manpageHTML = htmlContent
				found = true
				break
			}
		}
	}

	if !found {
		manpageHTML = fmt.Sprintf("<p>Manual page for <strong>%s</strong> not found.</p><p>Man page files should be located in the <code>doc/</code> directory.</p>", app)
	}

	// Get base template data
	data := s.getBaseTemplateData(c)
	data["App"] = app
	data["Manpage"] = template.HTML(manpageHTML)  // Mark as safe HTML to prevent escaping
	data["Title"] = fmt.Sprintf("Manual for %s", app)

	c.HTML(http.StatusOK, "manpage.html", data)
}

// Settings Endpoints

// handleLanguage changes the session language
func (s *Server) handleLanguage(c *gin.Context) {
	locale := c.PostForm("locale")
	if locale == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Locale is required",
		})
		return
	}

	// Store locale in session
	session, err := s.getSession(c)
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to get session: %v", err),
		})
		return
	}

	session.Values["language"] = locale
	if err := session.Save(c.Request, c.Writer); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to save session: %v", err),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Changed Web Interface language to %s", locale),
	})
}

// handleTheme changes the UI theme
func (s *Server) handleTheme(c *gin.Context) {
	var theme string
	if c.Request.Method == "GET" {
		theme = c.Query("theme")
	} else {
		theme = c.PostForm("theme")
	}

	if theme == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Theme is required",
		})
		return
	}

	// Validate theme
	validThemes := map[string]bool{
		"modern":  true,
		"classic": true,
	}

	if !validThemes[theme] {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "The requested theme does not exist.",
		})
		return
	}

	// Store theme in session
	session, err := s.getSession(c)
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to get session: %v", err),
		})
		return
	}

	session.Values["theme"] = theme
	if err := session.Save(c.Request, c.Writer); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to save session: %v", err),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("Theme changed to '%s'.", theme),
	})
}

// handlePWA serves PWA resources
func (s *Server) handlePWA(c *gin.Context) {
	pwaPath := c.Param("pwa_path")

	// Try embedded files first
	data, err := web.GetPWAFile(pwaPath)
	if err == nil {
		// Determine content type based on file extension
		contentType := "application/octet-stream"
		if strings.HasSuffix(pwaPath, ".json") {
			contentType = "application/json"
		} else if strings.HasSuffix(pwaPath, ".xml") {
			contentType = "application/xml"
		} else if strings.HasSuffix(pwaPath, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(pwaPath, ".ico") {
			contentType = "image/x-icon"
		}
		c.Data(http.StatusOK, contentType, data)
		return
	}

	// Fallback to filesystem for development
	fullPath := filepath.Join(s.config.StaticDir, "pwa", pwaPath)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"status": false,
			"msg":    "PWA resource not found",
		})
		return
	}

	c.File(fullPath)
}

// Advanced File Operations

// handleFilesDownloadURL downloads a file from a URL to the images directory
func (s *Server) handleFilesDownloadURL(c *gin.Context) {
	url := c.PostForm("url")
	destination := c.DefaultPostForm("destination", "disk_images")

	if url == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "URL is required",
		})
		return
	}

	// Determine destination directory
	var destinationDir string
	if destination == "disk_images" {
		destinationDir = s.config.BaseDir
	} else if destination == "shared_files" {
		destinationDir = s.config.SharedDir
	} else {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Unknown destination",
		})
		return
	}

	// Extract filename from URL
	fileName := filepath.Base(url)
	if !isValidFilename(fileName) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename in URL",
		})
		return
	}

	fullPath := filepath.Join(destinationDir, fileName)

	// Download file using wget or curl
	cmd := exec.Command("wget", "-O", fullPath, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try curl if wget fails
		cmd = exec.Command("curl", "-o", fullPath, "-L", url)
		output, err = cmd.CombinedOutput()
		if err != nil {
			s.respond(c, ResponseOptions{
				Error:   true,
				Message: fmt.Sprintf("Failed to download file: %v - %s", err, string(output)),
			})
			return
		}
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("File downloaded successfully: %s", fileName),
	})
}

// handleFilesCreateISO creates an ISO file from files or a directory
func (s *Server) handleFilesCreateISO(c *gin.Context) {
	isoName := c.PostForm("iso_name")
	volumeName := c.PostForm("volume_name")
	sourcePath := c.PostForm("source_path") // Can be a directory or file

	if isoName == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "ISO name is required",
		})
		return
	}

	// Default volume name to iso name if not provided
	if volumeName == "" {
		volumeName = isoName
	}

	// Default source path to current directory if not provided
	if sourcePath == "" {
		sourcePath = "."
	}

	// Validate filename
	if !isValidFilename(isoName) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid ISO filename",
		})
		return
	}

	// Ensure .iso extension
	if !strings.HasSuffix(strings.ToLower(isoName), ".iso") {
		isoName += ".iso"
	}

	isoPath := filepath.Join(s.config.BaseDir, isoName)
	fullSourcePath := filepath.Join(s.config.BaseDir, sourcePath)

	// Check if source exists
	if _, err := os.Stat(fullSourcePath); os.IsNotExist(err) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Source path not found: %s", sourcePath),
		})
		return
	}

	// Check if genisoimage is available
	if _, err := exec.LookPath("genisoimage"); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "genisoimage command not found. Please install genisoimage package.",
		})
		return
	}

	// Create ISO using genisoimage
	cmd := exec.Command("genisoimage",
		"-V", volumeName,
		"-o", isoPath,
		fullSourcePath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to create ISO: %v - %s", err, string(output)),
		})
		return
	}

	s.respond(c, ResponseOptions{
		Message: fmt.Sprintf("ISO file created successfully: %s", isoName),
		Data: gin.H{
			"file": isoName,
		},
	})
}

// handleFilesExtractImage extracts files from an archive
func (s *Server) handleFilesExtractImage(c *gin.Context) {
	archiveFile := c.PostForm("archive_file")
	archiveMembers := c.PostForm("archive_members") // Optional: pipe-separated list of members to extract

	if archiveFile == "" {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Archive file name is required",
		})
		return
	}

	// Validate filename
	if !isValidFilename(archiveFile) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "Invalid filename",
		})
		return
	}

	// Full path
	fullPath := filepath.Join(s.config.BaseDir, archiveFile)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Archive file not found: %s", archiveFile),
		})
		return
	}

	// Check if unar is available
	if _, err := exec.LookPath("unar"); err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: "unar command not found. Please install unar/The Unarchiver package.",
		})
		return
	}

	// Build unar command arguments
	args := []string{
		"-output-directory", s.config.BaseDir,
		"-force-skip",        // Skip existing files
		"-no-directory",      // Don't create subdirectory
		"-forks", "visible",  // Extract resource forks as .rsrc files
		"--",
		fullPath,
	}

	// Add specific members if requested
	if archiveMembers != "" {
		members := strings.Split(archiveMembers, "|")
		args = append(args, members...)
	}

	// Execute unar
	cmd := exec.Command("unar", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.respond(c, ResponseOptions{
			Error:   true,
			Message: fmt.Sprintf("Failed to extract archive: %v - %s", err, string(output)),
		})
		return
	}

	// Parse output to determine what was extracted
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	extractedCount := 0
	for _, line := range lines {
		// unar outputs "  filename... OK." for each extracted file
		if strings.Contains(line, "... OK.") {
			extractedCount++
		}
	}

	// Look for any .properties files that were extracted and could be moved
	// This is a simplified version - Python version does more sophisticated handling
	propertiesFiles := []string{}
	filepath.Walk(s.config.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".properties") {
			propertiesFiles = append(propertiesFiles, info.Name())
		}
		return nil
	})

	message := fmt.Sprintf("Successfully extracted %d file(s) from %s", extractedCount, archiveFile)
	if len(propertiesFiles) > 0 {
		message += fmt.Sprintf(". Found %d properties file(s).", len(propertiesFiles))
	}

	s.respond(c, ResponseOptions{
		Message: message,
		Data: gin.H{
			"extracted_count":  extractedCount,
			"properties_files": propertiesFiles,
		},
	})
}

// handleFilesUploadForm is an alternative upload endpoint (similar to regular upload)
func (s *Server) handleFilesUploadForm(c *gin.Context) {
	// Reuse the existing upload handler logic
	s.handleFilesUpload(c)
}
