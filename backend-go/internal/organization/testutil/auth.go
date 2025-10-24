package testutil

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// MockJWTSecret is the secret used for test JWT tokens
const MockJWTSecret = "test-jwt-secret-key-for-testing-only"

// GenerateTestJWT generates a test JWT token for a user
func GenerateTestJWT(userID, username, role string, expiration time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"role":     role,
		"iat":      now.Unix(),
		"exp":      now.Add(expiration).Unix(),
		"jti":      uuid.New().String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(MockJWTSecret))
}

// GenerateTestJWTDefault generates a test JWT token with default 24-hour expiration
func GenerateTestJWTDefault(userID, username string) (string, error) {
	return GenerateTestJWT(userID, username, "user", 24*time.Hour)
}

// CreateAuthContext creates a context with user authentication
func CreateAuthContext(userID string) context.Context {
	ctx := context.Background()
	return context.WithValue(ctx, "user_id", userID)
}

// CreateAuthContextWithEmail creates a context with user authentication and email
func CreateAuthContextWithEmail(userID, email string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userID)
	return context.WithValue(ctx, "user_email", email)
}

// TestAuthContext is a helper struct for creating authenticated contexts
type TestAuthContext struct {
	UserID   string
	Email    string
	Username string
	Role     string
}

// NewTestAuthContext creates a new test auth context helper
func NewTestAuthContext(userID, email, username string) *TestAuthContext {
	return &TestAuthContext{
		UserID:   userID,
		Email:    email,
		Username: username,
		Role:     "user",
	}
}

// WithRole sets the role for the auth context
func (t *TestAuthContext) WithRole(role string) *TestAuthContext {
	t.Role = role
	return t
}

// ToContext converts to a context.Context with values
func (t *TestAuthContext) ToContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "user_id", t.UserID)
	ctx = context.WithValue(ctx, "user_email", t.Email)
	ctx = context.WithValue(ctx, "username", t.Username)
	ctx = context.WithValue(ctx, "role", t.Role)
	return ctx
}

// GenerateToken generates a JWT token for this auth context
func (t *TestAuthContext) GenerateToken() (string, error) {
	return GenerateTestJWT(t.UserID, t.Username, t.Role, 24*time.Hour)
}

// MockAuthMiddleware provides helper methods for testing authentication
type MockAuthMiddleware struct {
	// Can be extended with more fields as needed
}

// NewMockAuthMiddleware creates a new mock auth middleware
func NewMockAuthMiddleware() *MockAuthMiddleware {
	return &MockAuthMiddleware{}
}

// ValidateToken mocks token validation
func (m *MockAuthMiddleware) ValidateToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(MockJWTSecret), nil
	})

	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if userID, ok := claims["user_id"].(string); ok {
			return userID, nil
		}
	}

	return "", jwt.ErrInvalidKey
}
