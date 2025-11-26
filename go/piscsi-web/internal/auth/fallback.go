package auth

import (
	"fmt"
	"os"
)

// FallbackAuthenticator provides simple password-based authentication
// for development/testing when PAM is not available
type FallbackAuthenticator struct {
	credentials map[string]string
}

// NewFallbackAuthenticator creates a fallback authenticator
func NewFallbackAuthenticator() *FallbackAuthenticator {
	credentials := map[string]string{
		"pi": "piscsi", // Default
	}

	// Allow setting custom credentials via environment variables
	if username := os.Getenv("FALLBACK_USER"); username != "" {
		password := os.Getenv("FALLBACK_PASSWORD")
		if password == "" {
			password = "password" // Default password if not specified
		}
		credentials[username] = password
	}

	return &FallbackAuthenticator{
		credentials: credentials,
	}
}

// Authenticate checks username and password against hardcoded list
func (f *FallbackAuthenticator) Authenticate(username, password string) error {
	expectedPassword, ok := f.credentials[username]
	if !ok {
		return fmt.Errorf("user not found")
	}

	if password != expectedPassword {
		return fmt.Errorf("invalid password")
	}

	return nil
}

// CheckGroupMembership always returns nil for fallback (no group checking)
func (f *FallbackAuthenticator) CheckGroupMembership(username string) error {
	// In fallback mode, we don't check group membership
	_, _ = fmt.Fprintf(os.Stderr, "WARNING: Using fallback authentication, group membership not checked\n")
	return nil
}

// AuthenticateAndAuthorize performs authentication only in fallback mode
func (f *FallbackAuthenticator) AuthenticateAndAuthorize(username, password string) error {
	return f.Authenticate(username, password)
}
