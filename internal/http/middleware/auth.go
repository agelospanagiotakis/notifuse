package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/golang-jwt/jwt/v5"
)

// writeJSONError writes a JSON error response with the given message and status code
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// SetPasswordPath is the one route an account with must_change_password set may reach.
const SetPasswordPath = "/api/user.setPassword"

// PasswordChangeCode is the machine-readable reason on the refusal, carried beside the
// human-readable error so the console can act on it without matching English prose. The
// console shows prose in three languages; a substring match would break in two of them.
const PasswordChangeCode = "password_change_required"

// passwordChangeGate answers "must this user replace their password before doing anything
// else?". It is a package-level seam rather than a field on AuthConfig because every handler
// builds its OWN AuthConfig in RegisterRoutes — there are 36 such constructions — and the one
// thing that must not be forgotten is wiring the gate into all of them. A field would have to
// be passed at every call site, and the failure mode of missing one is silent: that route
// simply stays usable with an administrator-issued password, which is the exact bug the gate
// exists to prevent. Registration is process-wide and happens once, from app.go, before the
// HTTP server starts.
//
// It is nil until registered, and a nil gate enforces nothing — which is what every unit test
// that exercises RequireAuth without a database expects.
var (
	passwordChangeGateMu sync.RWMutex
	passwordChangeGate   func(ctx context.Context, userID string) (bool, error)
)

// SetPasswordChangeGate installs the gate's data source. Passing nil removes it.
func SetPasswordChangeGate(fn func(ctx context.Context, userID string) (bool, error)) {
	passwordChangeGateMu.Lock()
	defer passwordChangeGateMu.Unlock()
	passwordChangeGate = fn
}

// getPasswordChangeGate reads the registered gate. It holds the read lock only long enough to
// copy the function value, so a request never runs database code while holding it.
func getPasswordChangeGate() func(ctx context.Context, userID string) (bool, error) {
	passwordChangeGateMu.RLock()
	defer passwordChangeGateMu.RUnlock()
	return passwordChangeGate
}

// writePasswordChangeRequired refuses the request with a status a client can distinguish from
// an ordinary authorization failure. 403 rather than 401: the credential is valid and the user
// is who they claim to be — there is one thing they have not done yet.
func writePasswordChangeRequired(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "Set a new password before continuing",
		"code":  PasswordChangeCode,
	})
}

// AuthConfig holds the configuration for the auth middleware
type AuthConfig struct {
	GetJWTSecret func() ([]byte, error)
}

// NewAuthMiddleware creates a new auth middleware with the given JWT secret provider
func NewAuthMiddleware(getJWTSecret func() ([]byte, error)) *AuthConfig {
	return &AuthConfig{
		GetJWTSecret: getJWTSecret,
	}
}

// RequireAuth creates a middleware that verifies the JWT token and user session
func (ac *AuthConfig) RequireAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSONError(w, "Authorization header is required", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				writeJSONError(w, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// Get JWT secret
			secret, err := ac.GetJWTSecret()
			if err != nil {
				writeJSONError(w, "Authentication unavailable", http.StatusServiceUnavailable)
				return
			}

			// Parse and validate token with claims
			claims := &service.UserClaims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				// CRITICAL: Verify signing method to prevent algorithm confusion
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return secret, nil
			})

			// CRITICAL: Check both error AND token.Valid
			if err != nil {
				writeJSONError(w, fmt.Sprintf("Invalid token: %v", err), http.StatusUnauthorized)
				return
			}
			if !token.Valid {
				writeJSONError(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			// Validate required claims
			if claims.UserID == "" {
				writeJSONError(w, "User ID not found in token", http.StatusUnauthorized)
				return
			}
			if claims.Type == "" {
				writeJSONError(w, "User type not found in token", http.StatusUnauthorized)
				return
			}
			if claims.Type == string(domain.UserTypeUser) && claims.SessionID == "" {
				writeJSONError(w, "Session ID not found in token", http.StatusUnauthorized)
				return
			}

			// Set context values
			ctx := context.WithValue(r.Context(), domain.UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, domain.UserTypeKey, claims.Type)
			if claims.Type == string(domain.UserTypeUser) {
				ctx = context.WithValue(ctx, domain.SessionIDKey, claims.SessionID)
			}

			// An administrator-issued password is a credential, not a decision: the person it
			// was issued to must replace it before they can do anything else. The check sits
			// here, in front of every authenticated route, rather than at each handler, because
			// a route that forgets it is a route that stays operable with a password that has
			// been read aloud over the phone.
			//
			// Only user sessions are gated. An API key has no password to change, and its
			// bearer token is unaffected by a member's credential.
			if claims.Type == string(domain.UserTypeUser) && r.URL.Path != SetPasswordPath {
				if gate := getPasswordChangeGate(); gate != nil {
					mustChange, err := gate(ctx, claims.UserID)
					if err != nil {
						// A gate that cannot answer must refuse, not allow. Answering 503
						// rather than 401 keeps the console's session: it recreates the stored
						// token on any 401, which would look like the user was signed out.
						writeJSONError(w, "Authentication is temporarily unavailable", http.StatusServiceUnavailable)
						return
					}
					if mustChange {
						writePasswordChangeRequired(w)
						return
					}
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RestrictedInDemo creates a middleware that returns a 400 error if the server is in demo mode
//
// Usage example:
//
//	restrictedMiddleware := middleware.RestrictedInDemo(true)
//	mux.Handle("/api/sensitive.operation", restrictedMiddleware(http.HandlerFunc(handler)))
//
// This middleware should be applied to endpoints that should be disabled in demo environments,
// such as operations that modify critical data or perform destructive actions.
func RestrictedInDemo(isDemo bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isDemo {
				writeJSONError(w, "This operation is not allowed in demo mode", http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
