package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// mintGateToken signs the bearer token a signed-in console user holds.
func mintGateToken(t *testing.T, userID, userType, sessionID string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id": userID,
		"type":    userType,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"nbf":     time.Now().Unix(),
	}
	if sessionID != "" {
		claims["session_id"] = sessionID
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(testJWTSecret)
	require.NoError(t, err)
	return signed
}

// registerGate installs a gate for the duration of one test and removes it afterwards.
// The gate is process-wide by design, so leaving one installed would reach the next test
// in the package and make it pass or fail for reasons it does not own.
func registerGate(t *testing.T, fn func(ctx context.Context, userID string) (bool, error)) {
	t.Helper()
	SetPasswordChangeGate(fn)
	t.Cleanup(func() { SetPasswordChangeGate(nil) })
}

func serveThroughGate(t *testing.T, path, token, userType, sessionID string) (*httptest.ResponseRecorder, *bool) {
	t.Helper()
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	authConfig := NewAuthMiddleware(func() ([]byte, error) { return testJWTSecret, nil })
	handler := authConfig.RequireAuth()(next)

	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w, &reached
}

func TestRequireAuth_PasswordChangeGate(t *testing.T) {
	const userID = "user-1"
	const sessionID = "session-1"
	userToken := mintGateToken(t, userID, string(domain.UserTypeUser), sessionID)

	t.Run("no gate registered enforces nothing", func(t *testing.T) {
		SetPasswordChangeGate(nil)
		w, reached := serveThroughGate(t, "/api/contacts.list", userToken, string(domain.UserTypeUser), sessionID)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, *reached)
	})

	t.Run("every route but the password route is refused while the flag is set", func(t *testing.T) {
		registerGate(t, func(_ context.Context, id string) (bool, error) {
			assert.Equal(t, userID, id)
			return true, nil
		})

		w, reached := serveThroughGate(t, "/api/contacts.list", userToken, string(domain.UserTypeUser), sessionID)

		require.Equal(t, http.StatusForbidden, w.Code)
		assert.False(t, *reached, "the request must not reach the handler")
		var body map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		// The console branches on the code, not on the English sentence: it renders prose in
		// three languages and a substring match would hold in one of them.
		assert.Equal(t, PasswordChangeCode, body["code"])
	})

	t.Run("the password route itself is reachable", func(t *testing.T) {
		registerGate(t, func(context.Context, string) (bool, error) { return true, nil })

		w, reached := serveThroughGate(t, SetPasswordPath, userToken, string(domain.UserTypeUser), sessionID)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, *reached, "the one route that can clear the flag must not be gated by it")
	})

	t.Run("a cleared flag lets the request through", func(t *testing.T) {
		registerGate(t, func(context.Context, string) (bool, error) { return false, nil })

		w, reached := serveThroughGate(t, "/api/contacts.list", userToken, string(domain.UserTypeUser), sessionID)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, *reached)
	})

	t.Run("an unanswered gate refuses rather than allows", func(t *testing.T) {
		registerGate(t, func(context.Context, string) (bool, error) {
			return false, errors.New("database is down")
		})

		w, reached := serveThroughGate(t, "/api/contacts.list", userToken, string(domain.UserTypeUser), sessionID)

		// 503, not 401: the console discards its stored token on a 401, which would sign the
		// user out and lose the page they were on over a transient database failure.
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.False(t, *reached)
	})

	t.Run("an API key is not gated", func(t *testing.T) {
		called := false
		registerGate(t, func(context.Context, string) (bool, error) {
			called = true
			return true, nil
		})
		keyToken := mintGateToken(t, "key-1", string(domain.UserTypeAPIKey), "")

		w, reached := serveThroughGate(t, "/api/contacts.list", keyToken, string(domain.UserTypeAPIKey), "")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, *reached)
		assert.False(t, called, "an API key has no password, so the gate must not be consulted")
	})
}
