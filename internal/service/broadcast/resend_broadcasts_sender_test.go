package broadcast

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResendBroadcastsAPIHelpers(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		assert.Equal(t, "Bearer re_test", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/audiences":
			_, _ = w.Write([]byte(`{"id":"aud_1"}`))
		case "/audiences/aud_1/contacts":
			w.WriteHeader(http.StatusCreated)
		case "/broadcasts":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			assert.Equal(t, "aud_1", body["audience_id"])
			_, _ = w.Write([]byte(`{"id":"bc_1"}`))
		case "/broadcasts/bc_1/send":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := &resendBroadcastsSender{client: srv.Client(), apiBase: srv.URL}
	ctx := context.Background()
	audience, err := s.createAudience(ctx, "re_test", "broadcast_1")
	require.NoError(t, err)
	assert.Equal(t, "aud_1", audience)

	require.NoError(t, s.addAudienceContacts(ctx, "re_test", audience, nil))
	bc, err := s.createBroadcast(ctx, "re_test", audience, "KOIS <info@example.com>", "Subject", "<p>Hi</p>", "Test", "reply@example.com")
	require.NoError(t, err)
	assert.Equal(t, "bc_1", bc)
	require.NoError(t, s.sendBroadcast(ctx, "re_test", bc))
	assert.Contains(t, paths, "POST /audiences")
	assert.Contains(t, paths, "POST /broadcasts")
	assert.Contains(t, paths, "POST /broadcasts/bc_1/send")
}
