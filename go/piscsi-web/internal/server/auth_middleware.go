package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

const (
	sessionName   = "piscsi_session"
	sessionUserKey = "username"
	sessionAuthKey = "authenticated"
)

// authMiddleware checks if the user is authenticated
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := s.sessionStore.Get(c.Request, sessionName)
		if err != nil {
			s.logger.Error("Failed to get session", "error", err)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Check if user is authenticated
		auth, ok := session.Values[sessionAuthKey].(bool)
		if !ok || !auth {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Store username in context for handlers to use
		if username, ok := session.Values[sessionUserKey].(string); ok {
			c.Set("username", username)
		}

		c.Next()
	}
}

// getUsername retrieves the username from the context
func getUsername(c *gin.Context) string {
	if username, exists := c.Get("username"); exists {
		if str, ok := username.(string); ok {
			return str
		}
	}
	return ""
}

// getSession retrieves the session for a request
// This is a helper method for handlers that need to access the session
func (s *Server) getSession(c *gin.Context) (*sessions.Session, error) {
	return s.sessionStore.Get(c.Request, sessionName)
}
