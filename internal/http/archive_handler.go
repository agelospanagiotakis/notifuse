package http

import (
	"crypto/subtle"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// ArchiveHandler serves the public campaign archive: /archive/{workspace}/ and
// /archive/{workspace}/{slug}.
//
// This is the Mailchimp campaign-archive surface, rebuilt on our own data. It is
// deliberately not the console and deliberately not the blog:
//
//   - the console is authenticated, so it cannot be the page a customer finds;
//   - the blog is a themed publication whose posts reference web-channel
//     templates, which we do not have for 182 imported campaigns, and whose home
//     page is a marketing page rather than a list of everything ever sent.
//
// What the archive serves is the sent campaign itself, compiled from the template
// version that was sent, so the page and the email cannot drift apart.
type ArchiveHandler struct {
	workspaceRepo domain.WorkspaceRepository
	templateRepo  domain.TemplateRepository
	archiveRepo   domain.CampaignArchiveRepository
	logger        logger.Logger
}

func NewArchiveHandler(
	workspaceRepo domain.WorkspaceRepository,
	templateRepo domain.TemplateRepository,
	archiveRepo domain.CampaignArchiveRepository,
	logger logger.Logger,
) *ArchiveHandler {
	return &ArchiveHandler{
		workspaceRepo: workspaceRepo,
		templateRepo:  templateRepo,
		archiveRepo:   archiveRepo,
		logger:        logger,
	}
}

func (h *ArchiveHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/archive", h.handle)
	mux.HandleFunc("/archive/", h.handle)
}

func (h *ArchiveHandler) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/archive"), "/")
	parts := []string{}
	if rest != "" {
		parts = strings.Split(rest, "/")
	}
	if len(parts) == 0 {
		http.Error(w, "archive: name a workspace, e.g. /archive/koisoptics/", http.StatusNotFound)
		return
	}

	workspace, err := h.resolveWorkspace(r, parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	settings := workspace.Settings.Archive
	if settings == nil || !settings.Enabled {
		// Fail closed: a workspace that has not turned the archive on has none.
		http.NotFound(w, r)
		return
	}

	if !h.authorized(w, r, settings) {
		return
	}

	if h.archiveRepo.EnsureSchema(r.Context(), workspace.ID) != nil {
		http.Error(w, "archive unavailable", http.StatusInternalServerError)
		return
	}

	switch len(parts) {
	case 1:
		h.renderList(w, r, workspace, settings)
	case 2:
		h.renderCampaign(w, r, workspace, settings, parts[1])
	default:
		http.NotFound(w, r)
	}
}

// resolveWorkspace matches the first path segment against the workspace id or its
// name, case-insensitively, so /archive/koisoptics/ and /archive/Kois-Optics/ are
// the same archive.
func (h *ArchiveHandler) resolveWorkspace(r *http.Request, segment string) (*domain.Workspace, error) {
	if ws, err := h.workspaceRepo.GetByID(r.Context(), segment); err == nil && ws != nil {
		return ws, nil
	}

	workspaces, err := h.workspaceRepo.List(r.Context())
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(segment)
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		if strings.ToLower(ws.ID) == want || strings.ToLower(ws.Name) == want {
			return ws, nil
		}
		// "kois-optics" and "kois optics" should reach the same workspace as
		// "Kois Optics" — the name is what a person types in a URL.
		if slugify(ws.Name) == want {
			return ws, nil
		}
	}
	return nil, fmt.Errorf("workspace not found")
}

func slugify(name string) string {
	out := make([]rune, 0, len(name))
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			dash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if len(out) > 0 && !dash {
				out = append(out, '-')
				dash = true
			}
		}
	}
	return strings.TrimRight(string(out), "-")
}

// authorized enforces the temporary basic-auth gate. An empty user means the
// archive is public, which is the end state: clearing two settings fields turns
// the gate off without touching code.
func (h *ArchiveHandler) authorized(w http.ResponseWriter, r *http.Request, settings *domain.ArchiveSettings) bool {
	if settings.BasicAuthUser == "" {
		return true
	}
	user, pass, ok := r.BasicAuth()
	if ok &&
		subtle.ConstantTimeCompare([]byte(user), []byte(settings.BasicAuthUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(settings.BasicAuthPassword)) == 1 {
		return true
	}

	w.Header().Set("WWW-Authenticate", `Basic realm="campaign archive", charset="UTF-8"`)
	// noindex: the gate is temporary, so a crawler that finds the URL must not
	// cache the 401 or index anything behind it later.
	w.Header().Set("X-Robots-Tag", "noindex")
	http.Error(w, "authentication required", http.StatusUnauthorized)
	return false
}

func (h *ArchiveHandler) renderList(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, settings *domain.ArchiveSettings) {
	entries, err := h.archiveRepo.List(r.Context(), workspace.ID)
	if err != nil {
		h.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Error("Failed to list campaign archive")
		http.Error(w, "archive unavailable", http.StatusInternalServerError)
		return
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].SentAt.After(entries[j].SentAt) })

	title := settings.Title
	if title == "" {
		title = workspace.Name + " · Newsletter archive"
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<meta name=\"robots\" content=\"noindex\">")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>")
	b.WriteString(archiveCSS)
	b.WriteString("</head><body><main>")
	b.WriteString("<header><h1>" + html.EscapeString(title) + "</h1>")
	b.WriteString(fmt.Sprintf("<p class=\"count\">%d campaign%s</p></header>",
		len(entries), plural(len(entries))))

	if len(entries) == 0 {
		b.WriteString("<p class=\"empty\">No campaigns have been archived yet.</p>")
	} else {
		b.WriteString("<ol class=\"campaigns\">")
		var lastYear int
		for _, e := range entries {
			year := e.SentAt.Year()
			if year != lastYear {
				b.WriteString(fmt.Sprintf("</ol><h2 class=\"year\">%d</h2><ol class=\"campaigns\">", year))
				lastYear = year
			}
			b.WriteString("<li><a href=\"/archive/" + html.EscapeString(slugify(workspace.Name)) + "/" + e.Slug + "\">")
			b.WriteString("<time>" + e.SentAt.Format("2 January 2006") + "</time>")
			b.WriteString("<span>" + html.EscapeString(e.Title) + "</span>")
			b.WriteString("</a></li>")
		}
		b.WriteString("</ol>")
	}

	b.WriteString("<footer>Generated by the newsletter platform · <a href=\"/archive/")
	b.WriteString(html.EscapeString(slugify(workspace.Name)))
	b.WriteString("/\">permanent link</a></footer></main></body></html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(b.String()))
}

func (h *ArchiveHandler) renderCampaign(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, settings *domain.ArchiveSettings, slug string) {
	entry, err := h.archiveRepo.Get(r.Context(), workspace.ID, slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	tpl, err := h.templateRepo.GetTemplateByID(r.Context(), workspace.ID, entry.TemplateID, entry.TemplateVersion)
	if err != nil || tpl == nil {
		h.logger.WithField("workspace_id", workspace.ID).WithField("slug", slug).Error("Archived campaign has no template")
		http.Error(w, "this campaign's template is no longer available", http.StatusGone)
		return
	}

	htmlOut, err := compileForWeb(workspace, tpl, entry)
	if err != nil {
		h.logger.WithField("slug", slug).WithField("error", err.Error()).Error("Failed to compile archived campaign")
		http.Error(w, "this campaign could not be rendered", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(htmlOut))
}

// compileForWeb renders the sent template as a web page: the email channel, minus
// the open-tracking pixel (Channel "web" is what suppresses it) and with the
// per-contact values replaced by neutral text, because an archive page has no
// reader to personalise for.
func compileForWeb(workspace *domain.Workspace, tpl *domain.Template, entry *domain.ArchiveEntry) (string, error) {
	if tpl.Email == nil || tpl.Email.MjmlSource == nil || *tpl.Email.MjmlSource == "" {
		return "", fmt.Errorf("template %s has no MJML source", tpl.ID)
	}

	subject := entry.Subject
	if subject == "" {
		subject = entry.Title
	}

	data := map[string]any{
		"workspace":       map[string]any{"name": workspace.Name},
		"contact":         map[string]any{"email": "", "first_name": "", "last_name": ""},
		"subject":         subject,
		"subject_preview": entry.Subject,
		// An archive has no subscription to cancel; Mailchimp's does the same.
		"unsubscribe_url": "",
	}

	resp, err := notifuse_mjml.CompileTemplate(notifuse_mjml.CompileTemplateRequest{
		MjmlSource:     tpl.Email.MjmlSource,
		Subject:        &subject,
		Channel:        "web",
		TemplateData:   data,
		PreserveLiquid: false,
	})
	if err != nil {
		return "", err
	}
	if !resp.Success {
		if resp.Error != nil {
			return "", fmt.Errorf("%s", resp.Error.Message)
		}
		return "", fmt.Errorf("compilation failed")
	}
	if resp.HTML == nil {
		return "", fmt.Errorf("compilation produced no HTML")
	}
	return *resp.HTML, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// archiveCSS is inline and self-contained: the archive must render from a single
// response with no build step, no theme upload, and no external asset that can
// 404 — that is the failure mode the blog theme would have introduced here.
const archiveCSS = `<style>
:root{--ink:#12141f;--muted:#5a6178;--line:#e4e7f0;--brand:#4051b5;--bg:#f7f8fc}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
main{max-width:760px;margin:0 auto;padding:56px 20px 72px}
header{border-bottom:2px solid var(--ink);padding-bottom:18px;margin-bottom:8px}
h1{font-size:30px;line-height:1.2;margin:0 0 6px}
.count{margin:0;color:var(--muted);font-size:14px}
h2.year{font-size:13px;letter-spacing:.12em;text-transform:uppercase;color:var(--muted);margin:34px 0 10px;font-weight:600}
ol.campaigns{list-style:none;margin:0;padding:0}
ol.campaigns li{border-bottom:1px solid var(--line)}
ol.campaigns a{display:flex;gap:16px;align-items:baseline;padding:13px 4px;text-decoration:none;color:inherit}
ol.campaigns a:hover{background:#fff;color:var(--brand)}
ol.campaigns time{flex:none;width:150px;color:var(--muted);font-size:13px;font-variant-numeric:tabular-nums}
ol.campaigns span{flex:1;min-width:0}
.empty{color:var(--muted)}
footer{margin-top:40px;padding-top:16px;border-top:1px solid var(--line);color:var(--muted);font-size:13px}
footer a{color:var(--brand)}
@media(max-width:520px){ol.campaigns a{flex-direction:column;gap:2px}ol.campaigns time{width:auto}}
</style>`
