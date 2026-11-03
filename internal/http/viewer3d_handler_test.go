package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/pkg/logger"
)

func Test3DViewer_RendersTheModel(t *testing.T) {
	h := &RootHandler{logger: logger.NewLogger()}

	req := httptest.NewRequest(http.MethodGet,
		"/3d?src=https://minio-api.e-pragma.gr/koisnewsletters/_3d-verify/Astronaut.glb&name=Astronaut", nil)
	rec := httptest.NewRecorder()
	h.serve3DViewer(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `src="https://minio-api.e-pragma.gr/koisnewsletters/_3d-verify/Astronaut.glb"`)
	assert.Contains(t, body, "camera-controls", "dragging must rotate, which needs camera-controls")
	assert.Contains(t, body, "Astronaut")
	assert.Contains(t, body, "/console/model-viewer.min.js")
	assert.Equal(t, "noindex, nofollow", rec.Header().Get("X-Robots-Tag"))
}

func Test3DViewer_RefusesWhatItCannotRender(t *testing.T) {
	h := &RootHandler{logger: logger.NewLogger()}

	// Each case must embed nothing. The wording differs by cause — a missing src says so, a
	// refused scheme says that — and asserting one sentence for both is how a test starts
	// documenting the wrong behaviour.
	cases := map[string]struct{ target, want string }{
		"no src at all":       {"/3d", "Λείπει το μοντέλο"},
		"a javascript scheme": {"/3d?src=javascript:alert(1)", "δεν είναι έγκυρη"},
		"a bare path":         {"/3d?src=/etc/passwd", "δεν είναι έγκυρη"},
		// Go refuses a semicolon in a query string, so this one arrives empty; either answer
		// is safe, and what matters is that nothing is embedded.
		"a data url": {"/3d?src=data:model/gltf-binary;base64,AAAA", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.serve3DViewer(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))

			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			assert.NotContains(t, body, `<model-viewer id="mv"`, "nothing may be embedded")
			assert.NotContains(t, body, "javascript:")
			if tc.want != "" {
				assert.Contains(t, body, tc.want)
			}
		})
	}
}

func Test3DViewer_RejectsWrites(t *testing.T) {
	h := &RootHandler{logger: logger.NewLogger()}
	rec := httptest.NewRecorder()
	h.serve3DViewer(rec, httptest.NewRequest(http.MethodPost, "/3d?src=https://x/y.glb", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
