package http

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// viewer3DPage is the standalone GLB viewer.
//
// It exists because an email cannot rotate anything: mail clients strip JavaScript, so a model
// can only be *linked to*. This page is what that link opens — a model you can drag, zoom and
// fullscreen, served from this domain with no session, so it can be sent to a client or embedded
// in a web page.
//
// The element and its script come from the console's own copies (console/public/model-viewer.min.js,
// copied from node_modules at build time), so the page depends on no third-party CDN.
const viewer3DPage = `<!doctype html>
<html lang="el">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>{{if .Title}}{{.Title}} — 3D{{else}}3D model{{end}}</title>
<style>
  :root { color-scheme: light; }
  html, body { margin: 0; height: 100%; background: #eef0f4; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
  #stage { position: fixed; inset: 0; background: radial-gradient(circle at 50% 35%, #f8f9fb 0%, #e7e9ee 55%, #d7dae1 100%); }
  model-viewer { width: 100%; height: 100%; background: transparent; }
  #bar { position: fixed; left: 50%; bottom: 20px; transform: translateX(-50%); display: flex; gap: 8px; align-items: center;
         background: rgba(255,255,255,.92); border: 1px solid #d9dce3; border-radius: 999px; padding: 8px 12px;
         box-shadow: 0 6px 24px rgba(20,25,40,.12); backdrop-filter: blur(6px); }
  #bar button { border: 0; background: #f1f2f6; color: #2b2f3a; border-radius: 999px; padding: 7px 14px; font-size: 13px; cursor: pointer; }
  #bar button:hover { background: #e6e8ef; }
  #bar button[aria-pressed="true"] { background: #42495b; color: #fff; }
  #hint { position: fixed; top: 16px; left: 50%; transform: translateX(-50%); color: #6b7280; font-size: 13px; }
  #error { position: fixed; inset: 0; display: none; place-items: center; padding: 32px; text-align: center; color: #b42318; }
</style>
</head>
<body>
<div id="stage">
  {{if .Src}}
  <model-viewer id="mv" src="{{.Src}}" {{if .Title}}alt="{{.Title}}"{{end}} camera-controls touch-action="pan-y"
                shadow-intensity="0.35" exposure="1" interaction-prompt="auto" style="width:100%;height:100%"></model-viewer>
  {{end}}
</div>
{{if .Src}}
<div id="hint">Σύρετε για περιστροφή · scroll για zoom</div>
<div id="bar">
  <button id="rotate" aria-pressed="false">Auto-rotate</button>
  <button id="reset">Reset</button>
  <button id="full">Fullscreen</button>
</div>
{{end}}
<div id="error">{{if .Error}}{{.Error}}{{end}}</div>
<script type="module" src="/console/model-viewer.min.js"></script>
<script>
(function () {
  var mv = document.getElementById('mv');
  var err = document.getElementById('error');
  if (!mv) { err.style.display = 'grid'; return; }
  mv.addEventListener('error', function () {
    err.textContent = 'Το μοντέλο δεν φορτώθηκε. Ελέγξτε ότι το αρχείο είναι έγκυρο .glb και ότι ο χώρος αποθήκευσης επιτρέπει ανάγνωση από αυτόν τον ιστότοπο.';
    err.style.display = 'grid';
  });
  var rotate = document.getElementById('rotate');
  rotate.addEventListener('click', function () {
    var on = rotate.getAttribute('aria-pressed') !== 'true';
    rotate.setAttribute('aria-pressed', on ? 'true' : 'false');
    if (on) { mv.setAttribute('auto-rotate', ''); } else { mv.removeAttribute('auto-rotate'); }
  });
  document.getElementById('reset').addEventListener('click', function () {
    if (mv.resetTurntableRotation) mv.resetTurntableRotation();
    mv.cameraOrbit = '0deg 75deg auto';
    mv.jumpCameraToGoal && mv.jumpCameraToGoal();
  });
  document.getElementById('full').addEventListener('click', function () {
    var el = document.querySelector('#stage');
    if (document.fullscreenElement) { document.exitFullscreen(); }
    else if (el.requestFullscreen) { el.requestFullscreen(); }
  });
})();
</script>
</body>
</html>`

// serve3DViewer renders one GLB, addressed by ?src=.
func (h *RootHandler) serve3DViewer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	src := strings.TrimSpace(r.URL.Query().Get("src"))
	title := strings.TrimSpace(r.URL.Query().Get("name"))
	if len(title) > 120 {
		title = title[:120]
	}

	errMsg := ""
	if src == "" {
		errMsg = "Λείπει το μοντέλο: προσθέστε ?src=https://…/model.glb στη διεύθυνση."
		src = ""
	} else if u, err := url.Parse(src); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		// Only a real http(s) URL: the template escapes it, but a scheme it does not understand
		// (javascript:, data:) is refused outright rather than escaped and hoped for.
		errMsg = "Η διεύθυνση του μοντέλου δεν είναι έγκυρη."
		src = ""
	}

	data := struct {
		Src   string
		Title string
		Error string
	}{Src: src, Title: title, Error: errMsg}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	if err := template.Must(template.New("v3d").Parse(viewer3DPage)).Execute(w, data); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to render the 3D viewer")
	}
}
