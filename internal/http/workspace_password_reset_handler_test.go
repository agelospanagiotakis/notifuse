package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// postSetUserPassword drives the route the way the console does.
func postSetUserPassword(t *testing.T, mux *http.ServeMux, secretKey []byte, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPassword", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "owner-user"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestWorkspaceHandler_SetUserPassword(t *testing.T) {
	t.Run("returns the generated password once", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)

		workspaceSvc.EXPECT().
			SetUserPassword(gomock.Any(), domain.SetUserPasswordRequest{WorkspaceID: "acme", UserID: "user-1"}).
			Return(&domain.SetUserPasswordResponse{Password: "AbcDefGhiJkmNpqRstUv", MustChangePassword: true}, nil)

		w := postSetUserPassword(t, mux, secretKey, `{"workspace_id":"acme","user_id":"user-1"}`)

		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "AbcDefGhiJkmNpqRstUv", body["password"])
		// The console must be told what the server enforced, not left to assume it.
		assert.Equal(t, true, body["must_change_password"])
	})

	t.Run("a refusal is a 403 with the service's own message", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)

		workspaceSvc.EXPECT().
			SetUserPassword(gomock.Any(), gomock.Any()).
			Return(nil, &domain.ErrUnauthorized{Message: "only workspace owners can reset a member's password"})

		w := postSetUserPassword(t, mux, secretKey, `{"workspace_id":"acme","user_id":"user-1"}`)

		require.Equal(t, http.StatusForbidden, w.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "only workspace owners can reset a member's password", body["error"])
	})

	t.Run("an unknown target is a 404, not a 500", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)

		workspaceSvc.EXPECT().
			SetUserPassword(gomock.Any(), gomock.Any()).
			Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

		w := postSetUserPassword(t, mux, secretKey, `{"workspace_id":"acme","user_id":"ghost"}`)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("a malformed request is rejected before the service is reached", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)
		// No expectations on workspaceSvc: reaching it would fail this test.

		w := postSetUserPassword(t, mux, secretKey, `{"workspace_id":"","user_id":""}`)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		_ = workspaceSvc
	})

	t.Run("the wrong verb is refused", func(t *testing.T) {
		_, _, mux, secretKey, _ := setupTest(t)

		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.setUserPassword", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "owner-user"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("an unauthenticated call never reaches the service", func(t *testing.T) {
		_, _, mux, _, _ := setupTest(t)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setUserPassword",
			strings.NewReader(`{"workspace_id":"acme","user_id":"user-1"}`))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
