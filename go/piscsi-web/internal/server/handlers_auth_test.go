package server

import (
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

func TestHandleLogin_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create server with mock authenticator that accepts "admin/password"
	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore:  store,
		authenticator: testutil.NewMockAuthenticatorWithCredentials("admin", "password"),
		config:        &config.Config{},
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// Create test router and register login handler
	router := gin.New()
	router.POST("/login", server.handleLogin)

	// Create login request
	form := url.Values{}
	form.Add("username", "admin")
	form.Add("password", "password")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	// Verify redirect to index
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected status 302 (Found), got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("expected redirect to '/', got '%s'", location)
	}

	// Verify session cookie was set
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}

	// Verify session contains auth data
	sessionCookie := cookies[0]
	testReq := httptest.NewRequest("GET", "/", nil)
	testReq.AddCookie(sessionCookie)

	session, err := store.Get(testReq, sessionName)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if auth, ok := session.Values[sessionAuthKey].(bool); !ok || !auth {
		t.Error("expected session to have authenticated=true")
	}

	if username, ok := session.Values[sessionUserKey].(string); !ok || username != "admin" {
		t.Errorf("expected session username to be 'admin', got '%s'", username)
	}
}

func TestHandleLogin_InvalidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore:  store,
		authenticator: testutil.NewMockAuthenticatorWithCredentials("admin", "password"),
		config:        &config.Config{},
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.LoadHTMLGlob("../../web/templates/*.html")
	router.POST("/login", server.handleLogin)

	// Try to login with wrong password
	form := url.Values{}
	form.Add("username", "admin")
	form.Add("password", "wrongpassword")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	// Verify 401 Unauthorized
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 (Unauthorized), got %d", resp.StatusCode)
	}

	// Verify response contains error message
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Invalid username or password") {
		t.Error("expected response to contain error message about invalid credentials")
	}
}

func TestHandleLogin_MissingUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore:  store,
		authenticator: testutil.NewMockAuthenticatorAlwaysSuccess(),
		config:        &config.Config{},
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.LoadHTMLGlob("../../web/templates/*.html")
	router.POST("/login", server.handleLogin)

	// Try to login without username
	form := url.Values{}
	form.Add("password", "password")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	// Verify 400 Bad Request
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 (Bad Request), got %d", resp.StatusCode)
	}

	// Verify error message
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Username and password are required") {
		t.Error("expected response to contain error message about required fields")
	}
}

func TestHandleLogin_MissingPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore:  store,
		authenticator: testutil.NewMockAuthenticatorAlwaysSuccess(),
		config:        &config.Config{},
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.LoadHTMLGlob("../../web/templates/*.html")
	router.POST("/login", server.handleLogin)

	// Try to login without password
	form := url.Values{}
	form.Add("username", "admin")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	// Verify 400 Bad Request
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 (Bad Request), got %d", resp.StatusCode)
	}
}

func TestHandleLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/logout", server.handleLogout)

	// First create a session with authentication
	req := httptest.NewRequest("POST", "/logout", nil)
	session, _ := store.Get(req, sessionName)
	session.Values[sessionAuthKey] = true
	session.Values[sessionUserKey] = "testuser"

	w := httptest.NewRecorder()
	session.Save(req, w)

	// Get the session cookie
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("failed to create session")
	}

	// Now logout with the session cookie
	logoutReq := httptest.NewRequest("POST", "/logout", nil)
	logoutReq.AddCookie(cookies[0])
	logoutW := httptest.NewRecorder()

	router.ServeHTTP(logoutW, logoutReq)

	resp := logoutW.Result()

	// Verify redirect to login page
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected status 302 (Found), got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/login" {
		t.Errorf("expected redirect to '/login', got '%s'", location)
	}

	// Verify session was cleared
	logoutCookies := resp.Cookies()
	if len(logoutCookies) > 0 {
		verifyReq := httptest.NewRequest("GET", "/", nil)
		verifyReq.AddCookie(logoutCookies[0])

		verifySession, err := store.Get(verifyReq, sessionName)
		if err != nil {
			t.Fatalf("failed to get session after logout: %v", err)
		}

		if auth, ok := verifySession.Values[sessionAuthKey].(bool); ok && auth {
			t.Error("expected session to be unauthenticated after logout")
		}
	}
}

func TestHandleLogout_NoSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := sessions.NewCookieStore([]byte("test-secret-key"))
	server := &Server{
		sessionStore: store,
		config:       &config.Config{},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	router := gin.New()
	router.POST("/logout", server.handleLogout)

	// Logout without any session
	req := httptest.NewRequest("POST", "/logout", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()

	// Should still redirect to login even without session
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected status 302 (Found), got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/login" {
		t.Errorf("expected redirect to '/login', got '%s'", location)
	}
}
