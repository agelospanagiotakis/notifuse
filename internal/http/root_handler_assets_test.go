package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the fix for a bug that made every open tab unusable across a deploy:
// a request for a bundle hash the new image no longer has was answered with index.html,
// so the browser received HTML where it expected JavaScript, failed to parse it, and the
// app never mounted.
func newTestRootHandlerWithDirs(t *testing.T) (*RootHandler, string) {
	t.Helper()
	consoleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(consoleDir, "index.html"),
		[]byte("<!doctype html><html><body>shell</body></html>"), 0o644))
	assets := filepath.Join(consoleDir, "assets")
	require.NoError(t, os.MkdirAll(assets, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(assets, "index-abc123.js"),
		[]byte("console.log('app')"), 0o644))

	ncDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ncDir, "index.html"),
		[]byte("<!doctype html><html><body>nc</body></html>"), 0o644))

	return &RootHandler{
		consoleDir:            consoleDir,
		notificationCenterDir: ncDir,
		logger:                logger.NewLogger(),
	}, consoleDir
}

func TestServeConsole_MissingAssetIs404NotHTML(t *testing.T) {
	h, _ := newTestRootHandlerWithDirs(t)

	// The bundle hash from the previous image.
	req := httptest.NewRequest(http.MethodGet, "/console/assets/index-OLDHASH.js", nil)
	w := httptest.NewRecorder()
	h.serveConsole(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "a missing script must not be answered with the shell")
	assert.NotContains(t, w.Body.String(), "<!doctype html>",
		"returning HTML for a .js request is what leaves a tab stuck on a spinner")
}

func TestServeConsole_ExistingAssetServedAndCached(t *testing.T) {
	h, _ := newTestRootHandlerWithDirs(t)

	req := httptest.NewRequest(http.MethodGet, "/console/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	h.serveConsole(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "console.log('app')", w.Body.String())
	assert.Contains(t, w.Header().Get("Cache-Control"), "immutable",
		"content-hashed assets can be cached forever")
}

func TestServeConsole_UnknownRouteServesShellUncached(t *testing.T) {
	h, _ := newTestRootHandlerWithDirs(t)

	req := httptest.NewRequest(http.MethodGet, "/console/workspaces/epragma/broadcasts", nil)
	w := httptest.NewRecorder()
	h.serveConsole(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "shell", "client-side routes must still reach the app")
	assert.Contains(t, w.Header().Get("Cache-Control"), "no-cache",
		"the shell is the only file that knows which bundle hash is current; it must revalidate")
}

func TestIsStaticAssetPath(t *testing.T) {
	cases := map[string]bool{
		"/assets/index-abc.js": true,
		"/favicon.ico":         true,
		"/logo.svg":            true,
		"/workspaces/epragma":  false,
		"/settings/integrations": false,
		"/":                    false,
	}
	for p, want := range cases {
		assert.Equal(t, want, isStaticAssetPath(p), "path %q", p)
	}
}

func TestServeNotificationCenter_MissingAssetIs404(t *testing.T) {
	h, _ := newTestRootHandlerWithDirs(t)

	req := httptest.NewRequest(http.MethodGet, "/notification-center/assets/index-OLDHASH.js", nil)
	w := httptest.NewRecorder()
	h.serveNotificationCenter(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
