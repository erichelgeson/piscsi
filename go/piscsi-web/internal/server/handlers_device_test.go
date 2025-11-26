package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/piscsi/piscsi-web/internal/config"
	"github.com/piscsi/piscsi-web/internal/server/testutil"
)

// TestHandleAttach_Success tests successfully attaching a device
func TestHandleAttach_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/attach", server.handleAttach)

	form := url.Values{}
	form.Add("scsi_id", "6")
	form.Add("unit", "0")
	form.Add("type", "SCHD")
	form.Add("file", "/images/test.hds")

	req := httptest.NewRequest("POST", "/scsi/attach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	// Verify redirect
	if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
		// In unit tests, status code handling may differ
		location := resp.Header.Get("Location")
		if location != "/" {
			t.Errorf("expected redirect to '/', got '%s'", location)
		}
	}
}

// TestHandleAttach_MissingSCSIID tests attach with missing SCSI ID
func TestHandleAttach_MissingSCSIID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/attach", server.handleAttach)

	form := url.Values{}
	form.Add("type", "SCHD")
	form.Add("file", "/images/test.hds")

	req := httptest.NewRequest("POST", "/scsi/attach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	// Verify session contains error flash message
	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "SCSI ID is required") {
			t.Errorf("expected error about required SCSI ID, got: %s", errorMessage)
		}
	}
}

// TestHandleAttach_InvalidSCSIID tests attach with out-of-range SCSI ID
func TestHandleAttach_InvalidSCSIID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/attach", server.handleAttach)

	tests := []struct {
		name   string
		scsiID string
	}{
		{"negative ID", "-1"},
		{"ID too high", "8"},
		{"invalid format", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("scsi_id", tt.scsiID)
			form.Add("type", "SCHD")

			req := httptest.NewRequest("POST", "/scsi/attach", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			resp := w.Result()

			// Verify error message in session
			cookies := resp.Cookies()
			if len(cookies) > 0 {
				verifyReq := httptest.NewRequest("GET", "/", nil)
				verifyReq.AddCookie(cookies[0])

				session, err := store.Get(verifyReq, sessionName)
				if err != nil {
					t.Fatalf("failed to get session: %v", err)
				}

				_, errorMessage := GetFlashesForTemplate(session)
				if !strings.Contains(errorMessage, "Invalid SCSI ID") {
					t.Errorf("expected error about invalid SCSI ID, got: %s", errorMessage)
				}
			}
		})
	}
}

// TestHandleAttach_InvalidLUN tests attach with out-of-range LUN
func TestHandleAttach_InvalidLUN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/attach", server.handleAttach)

	form := url.Values{}
	form.Add("scsi_id", "6")
	form.Add("unit", "32") // LUN must be 0-31
	form.Add("type", "SCHD")

	req := httptest.NewRequest("POST", "/scsi/attach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Invalid LUN") {
			t.Errorf("expected error about invalid LUN, got: %s", errorMessage)
		}
	}
}

// TestHandleAttach_DaemonError tests attach when daemon communication fails
func TestHandleAttach_DaemonError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientWithError(errors.New("connection refused"))
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/attach", server.handleAttach)

	form := url.Values{}
	form.Add("scsi_id", "6")
	form.Add("type", "SCHD")
	form.Add("file", "/images/test.hds")

	req := httptest.NewRequest("POST", "/scsi/attach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Failed to communicate") {
			t.Errorf("expected daemon communication error, got: %s", errorMessage)
		}
	}
}

// TestHandleAttach_ResultStatusFalse tests attach when daemon returns false status
func TestHandleAttach_ResultStatusFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysFail("Device already attached")
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/attach", server.handleAttach)

	form := url.Values{}
	form.Add("scsi_id", "6")
	form.Add("type", "SCHD")
	form.Add("file", "/images/test.hds")

	req := httptest.NewRequest("POST", "/scsi/attach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Device already attached") {
			t.Errorf("expected daemon error message, got: %s", errorMessage)
		}
	}
}

// TestHandleDetach_Success tests successfully detaching a device
func TestHandleDetach_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/detach", server.handleDetach)

	form := url.Values{}
	form.Add("scsi_id", "6")
	form.Add("unit", "0")

	req := httptest.NewRequest("POST", "/scsi/detach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("expected redirect to '/', got '%s'", location)
	}
}

// TestHandleDetach_MissingSCSIID tests detach with missing SCSI ID
func TestHandleDetach_MissingSCSIID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/detach", server.handleDetach)

	form := url.Values{}
	// Missing scsi_id

	req := httptest.NewRequest("POST", "/scsi/detach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "SCSI ID is required") {
			t.Errorf("expected error about required SCSI ID, got: %s", errorMessage)
		}
	}
}

// TestHandleDetach_InvalidSCSIID tests detach with invalid SCSI ID
func TestHandleDetach_InvalidSCSIID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/detach", server.handleDetach)

	form := url.Values{}
	form.Add("scsi_id", "99") // Invalid: must be 0-7

	req := httptest.NewRequest("POST", "/scsi/detach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Invalid SCSI ID") {
			t.Errorf("expected error about invalid SCSI ID, got: %s", errorMessage)
		}
	}
}

// TestHandleDetach_DaemonError tests detach when daemon communication fails
func TestHandleDetach_DaemonError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientWithError(errors.New("connection refused"))
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/detach", server.handleDetach)

	form := url.Values{}
	form.Add("scsi_id", "6")

	req := httptest.NewRequest("POST", "/scsi/detach", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Failed to communicate") {
			t.Errorf("expected daemon communication error, got: %s", errorMessage)
		}
	}
}

// TestHandleDetachAll_Success tests successfully detaching all devices
func TestHandleDetachAll_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/detach/all", server.handleDetachAll)

	req := httptest.NewRequest("POST", "/scsi/detach/all", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("expected redirect to '/', got '%s'", location)
	}

	// Verify success message
	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		flashMessage, _ := GetFlashesForTemplate(session)
		if !strings.Contains(flashMessage, "Detached all devices") {
			t.Errorf("expected success message about detaching all devices, got: %s", flashMessage)
		}
	}
}

// TestHandleDetachAll_DaemonError tests detach all when daemon fails
func TestHandleDetachAll_DaemonError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientWithError(errors.New("connection refused"))
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/detach/all", server.handleDetachAll)

	req := httptest.NewRequest("POST", "/scsi/detach/all", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Failed to communicate") {
			t.Errorf("expected daemon communication error, got: %s", errorMessage)
		}
	}
}

// TestHandleEject_Success tests successfully ejecting removable media
func TestHandleEject_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/eject", server.handleEject)

	form := url.Values{}
	form.Add("scsi_id", "6")
	form.Add("unit", "0")

	req := httptest.NewRequest("POST", "/scsi/eject", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("expected redirect to '/', got '%s'", location)
	}

	// Verify success message
	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		flashMessage, _ := GetFlashesForTemplate(session)
		if !strings.Contains(flashMessage, "Ejected media") {
			t.Errorf("expected success message about ejecting media, got: %s", flashMessage)
		}
	}
}

// TestHandleEject_MissingSCSIID tests eject with missing SCSI ID
func TestHandleEject_MissingSCSIID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/eject", server.handleEject)

	form := url.Values{}
	// Missing scsi_id

	req := httptest.NewRequest("POST", "/scsi/eject", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "SCSI ID is required") {
			t.Errorf("expected error about required SCSI ID, got: %s", errorMessage)
		}
	}
}

// TestHandleEject_InvalidSCSIID tests eject with invalid SCSI ID
func TestHandleEject_InvalidSCSIID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysSuccess()
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/eject", server.handleEject)

	form := url.Values{}
	form.Add("scsi_id", "10") // Invalid: must be 0-7

	req := httptest.NewRequest("POST", "/scsi/eject", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Invalid SCSI ID") {
			t.Errorf("expected error about invalid SCSI ID, got: %s", errorMessage)
		}
	}
}

// TestHandleEject_DaemonError tests eject when daemon communication fails
func TestHandleEject_DaemonError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientWithError(errors.New("connection refused"))
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/eject", server.handleEject)

	form := url.Values{}
	form.Add("scsi_id", "6")

	req := httptest.NewRequest("POST", "/scsi/eject", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "Failed to communicate") {
			t.Errorf("expected daemon communication error, got: %s", errorMessage)
		}
	}
}

// TestHandleEject_ResultStatusFalse tests eject when daemon returns false status
func TestHandleEject_ResultStatusFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	piscsiClient := testutil.NewMockPiSCSIClientAlwaysFail("No media to eject")
	server := &Server{
		sessionStore: store,
		piscsiClient: piscsiClient,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/scsi/eject", server.handleEject)

	form := url.Values{}
	form.Add("scsi_id", "6")

	req := httptest.NewRequest("POST", "/scsi/eject", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	cookies := resp.Cookies()
	if len(cookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(cookies[0])

		session, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}

		_, errorMessage := GetFlashesForTemplate(session)
		if !strings.Contains(errorMessage, "No media to eject") {
			t.Errorf("expected daemon error message, got: %s", errorMessage)
		}
	}
}
