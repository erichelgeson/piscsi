package testutil

import "errors"

// ErrInvalidCredentials is returned when authentication fails
var ErrInvalidCredentials = errors.New("invalid credentials")

// MockAuthenticator is a mock authentication provider for testing
type MockAuthenticator struct {
	AuthenticateAndAuthorizeFunc func(username, password string) error
}

// AuthenticateAndAuthorize calls the mock function
func (m *MockAuthenticator) AuthenticateAndAuthorize(username, password string) error {
	if m.AuthenticateAndAuthorizeFunc != nil {
		return m.AuthenticateAndAuthorizeFunc(username, password)
	}
	return nil
}

// NewMockAuthenticator creates a new mock authenticator
func NewMockAuthenticator() *MockAuthenticator {
	return &MockAuthenticator{}
}

// NewMockAuthenticatorWithCredentials creates a mock that accepts specific credentials
func NewMockAuthenticatorWithCredentials(validUsername, validPassword string) *MockAuthenticator {
	return &MockAuthenticator{
		AuthenticateAndAuthorizeFunc: func(username, password string) error {
			if username == validUsername && password == validPassword {
				return nil
			}
			return ErrInvalidCredentials
		},
	}
}

// NewMockAuthenticatorAlwaysSuccess creates a mock that always succeeds
func NewMockAuthenticatorAlwaysSuccess() *MockAuthenticator {
	return &MockAuthenticator{
		AuthenticateAndAuthorizeFunc: func(username, password string) error {
			return nil
		},
	}
}

// NewMockAuthenticatorAlwaysFail creates a mock that always fails
func NewMockAuthenticatorAlwaysFail() *MockAuthenticator {
	return &MockAuthenticator{
		AuthenticateAndAuthorizeFunc: func(username, password string) error {
			return ErrInvalidCredentials
		},
	}
}
