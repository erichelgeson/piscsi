package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

func TestRespond_JSONMode_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	// Create a test HTTP request with Accept: application/json
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", nil)
	c.Request.Header.Set("Accept", "application/json")

	// Call respond with success message
	server.respond(c, ResponseOptions{
		Message: "Operation successful",
	})

	// Verify response
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Parse JSON response
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	// Verify JSON structure
	if result["status"] != "success" {
		t.Errorf("expected status 'success', got %v", result["status"])
	}
	if result["message"] != "Operation successful" {
		t.Errorf("expected message 'Operation successful', got %v", result["message"])
	}
}

func TestRespond_JSONMode_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", nil)
	c.Request.Header.Set("Accept", "application/json")

	server.respond(c, ResponseOptions{
		Error:   true,
		Message: "Operation failed",
	})

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if result["status"] != "error" {
		t.Errorf("expected status 'error', got %v", result["status"])
	}
	if result["message"] != "Operation failed" {
		t.Errorf("expected message 'Operation failed', got %v", result["message"])
	}
}

func TestRespond_JSONMode_WithData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", nil)
	c.Request.Header.Set("Accept", "application/json")

	server.respond(c, ResponseOptions{
		Message: "Success",
		Data: gin.H{
			"device_id": 1,
			"scsi_id":   6,
			"file_name": "test.hds",
		},
	})

	body, _ := io.ReadAll(w.Result().Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if result["status"] != "success" {
		t.Errorf("expected status 'success', got %v", result["status"])
	}
	if result["device_id"] != float64(1) {
		t.Errorf("expected device_id 1, got %v", result["device_id"])
	}
	if result["scsi_id"] != float64(6) {
		t.Errorf("expected scsi_id 6, got %v", result["scsi_id"])
	}
	if result["file_name"] != "test.hds" {
		t.Errorf("expected file_name 'test.hds', got %v", result["file_name"])
	}
}

func TestRespond_HTMLMode_RedirectWithFlash(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", nil)

	server.respond(c, ResponseOptions{
		Message: "Device attached successfully",
	})

	resp := w.Result()

	// Verify Location header is set (status code handling requires full Gin router in integration tests)
	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("expected redirect to '/', got '%s'", location)
	}

	// Verify session cookie was set
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}

	// Extract and verify flash message from session
	sessionCookie := cookies[0]
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)

	session, err := store.Get(req, "piscsi_session")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	flashMessage, _ := GetFlashesForTemplate(session)
	if !strings.Contains(flashMessage, "Device attached successfully") {
		t.Errorf("expected flash message to contain 'Device attached successfully', got '%s'", flashMessage)
	}
}

func TestRespond_HTMLMode_RedirectWithError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", nil)

	server.respond(c, ResponseOptions{
		Error:   true,
		Message: "Failed to attach device",
	})

	resp := w.Result()

	// Verify Location header is set (redirects even for errors)
	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("expected redirect to '/', got '%s'", location)
	}

	// Verify session cookie
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}

	// Extract and verify error message from session
	sessionCookie := cookies[0]
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)

	session, err := store.Get(req, "piscsi_session")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	_, errorMessage := GetFlashesForTemplate(session)
	if !strings.Contains(errorMessage, "Failed to attach device") {
		t.Errorf("expected error message to contain 'Failed to attach device', got '%s'", errorMessage)
	}
}

func TestRespond_HTMLMode_CustomRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", nil)

	server.respond(c, ResponseOptions{
		Message:     "Configuration saved",
		RedirectURL: "/config",
	})

	resp := w.Result()

	// Verify custom redirect URL
	location := resp.Header.Get("Location")
	if location != "/config" {
		t.Errorf("expected redirect to '/config', got '%s'", location)
	}
}

func TestRespond_HTMLMode_TemplateRender(t *testing.T) {
	t.Skip("Template rendering requires full server setup with template engine")
	// TODO: Implement integration test for template rendering mode
}

func TestRespond_CustomStatusCode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	tests := []struct {
		name       string
		opts       ResponseOptions
		wantStatus int
	}{
		{
			name: "custom 201 created",
			opts: ResponseOptions{
				Message:    "Resource created",
				StatusCode: http.StatusCreated,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "custom 404 not found",
			opts: ResponseOptions{
				Error:      true,
				Message:    "Not found",
				StatusCode: http.StatusNotFound,
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/test", nil)
			c.Request.Header.Set("Accept", "application/json")

			server.respond(c, tt.opts)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestRespond_DefaultStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
	}

	tests := []struct {
		name       string
		isError    bool
		wantStatus int
	}{
		{"success defaults to 200", false, http.StatusOK},
		{"error defaults to 400", true, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/test", nil)
			c.Request.Header.Set("Accept", "application/json")

			server.respond(c, ResponseOptions{
				Error:   tt.isError,
				Message: "test",
			})

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

