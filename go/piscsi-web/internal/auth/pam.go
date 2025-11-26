//go:build !nopam

package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/msteinert/pam"
)

// Authenticator handles user authentication and authorization
type Authenticator struct {
	serviceName string
	authGroup   string
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(serviceName, authGroup string) *Authenticator {
	if serviceName == "" {
		serviceName = "login"
	}
	if authGroup == "" {
		authGroup = "piscsi"
	}

	return &Authenticator{
		serviceName: serviceName,
		authGroup:   authGroup,
	}
}

// Authenticate checks if username and password are valid via PAM
func (a *Authenticator) Authenticate(username, password string) error {
	// Start PAM transaction
	t, err := pam.StartFunc(a.serviceName, username, func(s pam.Style, msg string) (string, error) {
		switch s {
		case pam.PromptEchoOff:
			// Password prompt
			return password, nil
		case pam.PromptEchoOn:
			// Username prompt
			return username, nil
		case pam.ErrorMsg:
			return "", fmt.Errorf("PAM error: %s", msg)
		case pam.TextInfo:
			return "", nil
		}
		return "", fmt.Errorf("unrecognized PAM message style")
	})
	if err != nil {
		return fmt.Errorf("failed to start PAM transaction: %w", err)
	}

	// Authenticate
	if err := t.Authenticate(0); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

// CheckGroupMembership checks if a user is a member of the configured group
func (a *Authenticator) CheckGroupMembership(username string) error {
	// Read /etc/group file
	file, err := os.Open("/etc/group")
	if err != nil {
		return fmt.Errorf("failed to open /etc/group: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Group format: groupname:password:gid:user1,user2,user3
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}

		groupName := parts[0]
		if groupName != a.authGroup {
			continue
		}

		// Check if username is in the member list
		members := strings.Split(parts[3], ",")
		for _, member := range members {
			if strings.TrimSpace(member) == username {
				return nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading /etc/group: %w", err)
	}

	return fmt.Errorf("user %s is not a member of group %s", username, a.authGroup)
}

// AuthenticateAndAuthorize performs both authentication and authorization
func (a *Authenticator) AuthenticateAndAuthorize(username, password string) error {
	// First, authenticate via PAM
	if err := a.Authenticate(username, password); err != nil {
		return err
	}

	// Then, check group membership
	if err := a.CheckGroupMembership(username); err != nil {
		return err
	}

	return nil
}

// IsAuthenticationAvailable checks if PAM authentication is available
func IsAuthenticationAvailable() bool {
	// Check if fallback auth is explicitly requested
	if os.Getenv("DISABLE_PAM") == "true" || os.Getenv("USE_FALLBACK_AUTH") == "true" {
		return false
	}

	// Try to check if we can read /etc/group (basic sanity check)
	_, err := os.Stat("/etc/group")
	return err == nil
}
