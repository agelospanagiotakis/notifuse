package http

import (
	"crypto/subtle"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

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
	listService   domain.ListService
	logger        logger.Logger
}

func NewArchiveHandler(
	workspaceRepo domain.WorkspaceRepository,
	templateRepo domain.TemplateRepository,
	archiveRepo domain.CampaignArchiveRepository,
	listService domain.ListService,
	logger logger.Logger,
) *ArchiveHandler {
	return &ArchiveHandler{
		workspaceRepo: workspaceRepo,
		templateRepo:  templateRepo,
		archiveRepo:   archiveRepo,
		listService:   listService,
		logger:        logger,
	}
}

func (h *ArchiveHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/archive", h.handle)
	mux.HandleFunc("/archive/", h.handle)
}

func (h *ArchiveHandler) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, HEAD, POST")
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

	switch {
	case len(parts) == 2 && parts[1] == "subscribe":
		h.handleSubscribePage(w, r, workspace, settings)
		return
	case len(parts) == 2 && (parts[1] == "feed.xml" || parts[1] == "feed"):
		h.renderFeed(w, r, workspace, settings, nil)
	case len(parts) == 1:
		h.renderList(w, r, workspace, settings, nil)
	case len(parts) == 3 && (parts[2] == "feed.xml" || parts[2] == "feed"):
		// An audience's own feed, mirroring its page.
		audience, err := h.archiveRepo.GetAudience(r.Context(), workspace.ID, parts[1])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		h.renderFeed(w, r, workspace, settings, audience)
	case len(parts) == 2:
		// A campaign and an audience are addressed the same way: one segment under the
		// workspace. The campaign is tried first and is unambiguous in practice — every slug
		// it produces begins with the send date — so an audience slug can never shadow one.
		entry, err := h.archiveRepo.Get(r.Context(), workspace.ID, parts[1])
		if err == nil {
			h.renderCampaign(w, r, workspace, settings, entry)
			return
		}
		audience, err := h.archiveRepo.GetAudience(r.Context(), workspace.ID, parts[1])
		if err == nil {
			h.renderList(w, r, workspace, settings, audience)
			return
		}
		http.NotFound(w, r)
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

// audienceNav renders the switch between the workspace's audiences, or nothing when there is
// only one. It exists because the campaigns are not one stream: 105 of the 182 imported issues
// went to one list and 69 to a different one, so "everything" and "this list" are genuinely
// different pages, and a reader who lands on the wrong one needs a way across.
func (h *ArchiveHandler) audienceNav(r *http.Request, workspace *domain.Workspace, current *domain.ArchiveAudience) string {
	audiences, err := h.archiveRepo.ListAudiences(r.Context(), workspace.ID)
	if err != nil {
		// The nav is a convenience; the page behind it is the product. A failure here renders
		// nothing rather than failing the archive.
		h.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Failed to list archive audiences")
		return ""
	}
	if len(audiences) < 2 {
		return ""
	}

	wsSlug := html.EscapeString(slugify(workspace.Name))
	var b strings.Builder
	b.WriteString(`<nav class="audiences"><span class="audiences-label">Audiences</span>`)
	allClass := "audience"
	if current == nil {
		allClass = "audience current"
	}
	b.WriteString(`<a class="` + allClass + `" href="/archive/` + wsSlug + `/">All issues</a>`)

	for _, a := range audiences {
		slug := a.Slug
		if slug == "" {
			// No registry row: address it by its list id rather than hiding it. GetAudience
			// accepts either, so this link resolves.
			slug = a.ListID
		}
		name := a.Name
		if name == "" {
			name = a.ListID
		}
		class := "audience"
		if current != nil && current.ListID == a.ListID {
			class = "audience current"
		}
		b.WriteString(`<a class="` + class + `" href="/archive/` + wsSlug + `/` + html.EscapeString(slug) + `/">`)
		b.WriteString(html.EscapeString(name))
		b.WriteString(fmt.Sprintf(` <span class="audience-count">%d</span></a>`, a.Campaigns))
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// audienceCSS styles the audience switch. Inline for the same reason the rest of the archive's
// styling is: these pages are compiled campaign documents with their own stylesheets.
const audienceCSS = `<style>
.archive-nav{max-width:760px;margin:0 auto 18px;padding:0 20px}
.audiences{display:flex;flex-wrap:wrap;gap:8px 10px;align-items:center;font:14px/1.4 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
.audiences-label{color:#6b7280;font-size:12px;text-transform:uppercase;letter-spacing:.06em;margin-right:4px}
.audience{display:inline-flex;align-items:center;gap:7px;padding:5px 12px;border:1px solid #dcdfe8;border-radius:999px;color:#2b2f45;text-decoration:none;background:#fff}
.audience:hover{border-color:#12132e}
.audience.current{background:#12132e;border-color:#12132e;color:#fff}
.audience-count{font-size:11px;opacity:.65}
</style>`

// renderList serves the archive's home, or one audience's home when audience is set.
func (h *ArchiveHandler) renderList(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, settings *domain.ArchiveSettings, audience *domain.ArchiveAudience) {
	var entries []*domain.ArchiveEntry
	var err error
	if audience != nil {
		entries, err = h.archiveRepo.ListByListID(r.Context(), workspace.ID, audience.ListID)
	} else {
		entries, err = h.archiveRepo.List(r.Context(), workspace.ID)
	}
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
	if audience != nil {
		// The audience's name first: a reader who followed a link to one list should see which
		// list they are looking at, not the workspace's generic archive heading.
		title = audience.Name + " · " + title
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<meta name=\"robots\" content=\"noindex\">")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>")
	b.WriteString(archiveCSS)
	b.WriteString(archiveBarCSS)
	b.WriteString(audienceCSS)
	b.WriteString(signupCSS)
	b.WriteString("</head><body>")
	b.WriteString(toolbarHTML(archiveBarOptions{
		workspaceSlug: slugify(workspace.Name),
		workspaceName: barName(workspace, settings),
		pageURL:       absoluteURL(r, r.URL.Path),
		subscribeURL:  subscribeTarget(workspace, settings),
		public:        settings.BasicAuthUser == "",
	}))
	b.WriteString("<main>")
	b.WriteString("<header><h1>" + html.EscapeString(title) + "</h1>")
	b.WriteString(fmt.Sprintf("<p class=\"count\">%d campaign%s</p></header>",
		len(entries), plural(len(entries))))

	// Off by default: see ArchiveSettings.ShowAudienceNav. An audience page is still
	// reachable by its own URL; this only decides whether the home page advertises the split.
	if settings.ShowAudienceNav {
		if nav := h.audienceNav(r, workspace, audience); nav != "" {
			b.WriteString(`<div class="archive-nav">` + nav + `</div>`)
		}
	}

	if len(settings.SubscribeListIDs) > 0 {
		b.WriteString(inlineSignupForm(slugify(workspace.Name)))
	}

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
			b.WriteString("<span>" + html.EscapeString(e.Title))
			// Reported by the platform that sent it. Zero means unknown, so it is
			// omitted rather than rendered as "0 opens", which would be a lie.
			if e.SentCount > 0 {
				b.WriteString(fmt.Sprintf("<small class=\"figures\">sent to %s subscribers · %s opened</small>",
					thousands(e.SentCount), thousands(e.UniqueOpens)))
			}
			b.WriteString("</span>")
			b.WriteString("</a></li>")
		}
		b.WriteString("</ol>")
	}

	b.WriteString("<footer>Generated by the newsletter platform · <a href=\"/archive/")
	b.WriteString(html.EscapeString(slugify(workspace.Name)))
	if audience != nil {
		b.WriteString("/" + html.EscapeString(audience.Slug))
	}
	b.WriteString("/\">permanent link</a></footer></main></body></html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(b.String()))
}

func (h *ArchiveHandler) renderCampaign(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, settings *domain.ArchiveSettings, entry *domain.ArchiveEntry) {
	slug := entry.Slug
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

	page := withToolbar(htmlOut, toolbarHTML(archiveBarOptions{
		workspaceSlug: slugify(workspace.Name),
		workspaceName: barName(workspace, settings),
		pageURL:       absoluteURL(r, r.URL.Path),
		subscribeURL:  subscribeTarget(workspace, settings),
		public:        settings.BasicAuthUser == "",
	}))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(page))
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

// thousands formats a count the way a reader expects, not the way Go does.
func thousands(n int) string {
	s := strconv.Itoa(n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, ch := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, ch)
	}
	return string(out)
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
ol.campaigns .figures{display:block;color:var(--muted);font-size:12px;margin-top:3px}
.empty{color:var(--muted)}
footer{margin-top:40px;padding-top:16px;border-top:1px solid var(--line);color:var(--muted);font-size:13px}
footer a{color:var(--brand)}
@media(max-width:520px){ol.campaigns a{flex-direction:column;gap:2px}ol.campaigns time{width:auto}}
</style>`

// ==================== archive chrome ====================
//
// Mailchimp's archive page carries a toolbar: Subscribe, Past Issues, RSS,
// Translate. It is the one part of their page that is not the campaign, and it is
// what makes the archive feel like a publication rather than a copy of an email.
// Ours is the same four controls, with one difference that has a reason:
//
// Translate is a link into Google's translate proxy, which fetches the page
// server-side. While the archive is behind basic auth that fetch gets a 401, so
// the control is omitted rather than offered and broken. It appears on its own
// the moment the gate comes off.

type archiveBarOptions struct {
	workspaceSlug string
	workspaceName string
	pageURL       string
	subscribeURL  string
	public        bool
}

// barName is the brand shown in the toolbar. A title like
// "Kois Optics · Newsletter archive" carries a suffix that belongs on the list page
// heading, not in a 40px bar; the workspace name is often a slug ("kois-optics").
func barName(workspace *domain.Workspace, settings *domain.ArchiveSettings) string {
	if settings != nil && settings.Title != "" {
		if idx := strings.Index(settings.Title, "·"); idx > 0 {
			return strings.TrimSpace(settings.Title[:idx])
		}
		return settings.Title
	}
	return workspace.Name
}

func subscribeTarget(workspace *domain.Workspace, settings *domain.ArchiveSettings) string {
	// One of our own public lists is the closest thing to Mailchimp: the reader
	// signs up without leaving the archive.
	if len(settings.SubscribeListIDs) > 0 {
		return "/archive/" + slugify(workspace.Name) + "/subscribe"
	}
	if settings.SubscribeURL != "" {
		return absoluteExternalURL(settings.SubscribeURL)
	}
	// Falling back to the workspace's own site beats hiding the control: an archive
	// with no way to subscribe is the one thing Mailchimp's never is.
	return absoluteExternalURL(workspace.Settings.WebsiteURL)
}

// absoluteExternalURL turns "www.example.gr" into "https://www.example.gr". A setting
// saved without a scheme otherwise resolves against the archive's own path and lands
// on a 404 like /archive/kois-optics/www.example.gr.
func absoluteExternalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "/") {
		return raw
	}
	return "https://" + raw
}

func absoluteURL(r *http.Request, path string) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + r.Host + path
}

func toolbarHTML(o archiveBarOptions) string {
	var b strings.Builder
	b.WriteString(`<div class="mca-bar"><div class="mca-bar-inner">`)
	b.WriteString(`<a class="mca-name" href="/archive/` + html.EscapeString(o.workspaceSlug) + `/">` +
		html.EscapeString(o.workspaceName) + `</a>`)
	b.WriteString(`<nav>`)
	if o.subscribeURL != "" {
		b.WriteString(`<a class="mca-cta" href="` + html.EscapeString(o.subscribeURL) + `">Subscribe</a>`)
	}
	b.WriteString(`<a href="/archive/` + html.EscapeString(o.workspaceSlug) + `/">Past Issues</a>`)
	b.WriteString(`<a href="/archive/` + html.EscapeString(o.workspaceSlug) + `/feed.xml">RSS</a>`)
	if o.public {
		translate := "https://translate.google.com/translate?sl=auto&tl=en&u=" + url.QueryEscape(o.pageURL)
		b.WriteString(`<a href="` + html.EscapeString(translate) + `" rel="noopener nofollow" target="_blank">Translate</a>`)
	}
	b.WriteString(`</nav></div></div>`)
	return b.String()
}

// withToolbar puts the bar at the top of the compiled campaign, which is a whole
// HTML document: insert after the opening <body> tag rather than parse it, and
// fall back to prepending if the document has no body (a fragment template).
func withToolbar(page, bar string) string {
	lower := strings.ToLower(page)
	if idx := strings.Index(lower, "<body"); idx != -1 {
		if end := strings.Index(lower[idx:], ">"); end != -1 {
			cut := idx + end + 1
			return page[:cut] + bar + archiveBarCSS + page[cut:]
		}
	}
	return bar + page
}

// renderFeed serves the archive as RSS 2.0, which is how Mailchimp-style archives
// are followed without an account.
func (h *ArchiveHandler) renderFeed(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, settings *domain.ArchiveSettings, audience *domain.ArchiveAudience) {
	var entries []*domain.ArchiveEntry
	var err error
	if audience != nil {
		entries, err = h.archiveRepo.ListByListID(r.Context(), workspace.ID, audience.ListID)
	} else {
		entries, err = h.archiveRepo.List(r.Context(), workspace.ID)
	}
	if err != nil {
		http.Error(w, "archive unavailable", http.StatusInternalServerError)
		return
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].SentAt.After(entries[j].SentAt) })

	slug := slugify(workspace.Name)
	audiencePath := ""
	title := settings.Title
	if title == "" {
		title = workspace.Name + " · Newsletter archive"
	}
	if audience != nil {
		audiencePath = "/" + audience.Slug
		title = audience.Name + " · " + title
	}
	home := absoluteURL(r, "/archive/"+slug+audiencePath+"/")

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom"><channel>`)
	// Readers like a self link: it is how a copied feed URL is re-discovered.
	b.WriteString(`<atom:link href="` + xmlEscape(absoluteURL(r, "/archive/"+slug+audiencePath+"/feed.xml")) +
		`" rel="self" type="application/rss+xml"/>`)
	b.WriteString("<title>" + xmlEscape(title) + "</title>")
	b.WriteString("<link>" + xmlEscape(home) + "</link>")
	b.WriteString("<description>" + xmlEscape("Every campaign sent by "+workspace.Name) + "</description>")
	b.WriteString("<language>el</language>")
	if len(entries) > 0 {
		b.WriteString("<lastBuildDate>" + entries[0].SentAt.Format(time.RFC1123Z) + "</lastBuildDate>")
	}
	for _, e := range entries {
		link := absoluteURL(r, "/archive/"+slug+"/"+e.Slug)
		b.WriteString("<item>")
		b.WriteString("<title>" + xmlEscape(e.Title) + "</title>")
		b.WriteString("<link>" + xmlEscape(link) + "</link>")
		b.WriteString("<guid isPermaLink=\"true\">" + xmlEscape(link) + "</guid>")
		b.WriteString("<pubDate>" + e.SentAt.Format(time.RFC1123Z) + "</pubDate>")
		b.WriteString("<description>" + xmlEscape(e.Subject) + "</description>")
		b.WriteString("</item>")
	}
	b.WriteString(`</channel></rss>`)

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(b.String()))
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// archiveBarCSS is inline so the bar survives in the campaign document, which has
// its own stylesheet and its own width assumptions.
const archiveBarCSS = `<style>
.mca-bar{position:relative;z-index:2147483647;background:#12132e;color:#eef0f6;font:13px/1.4 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
.mca-bar-inner{max-width:760px;margin:0 auto;padding:10px 20px;display:flex;gap:18px;align-items:center;justify-content:space-between;flex-wrap:wrap}
.mca-name{color:#fff;text-decoration:none;font-weight:600;letter-spacing:.01em}
.mca-bar nav{display:flex;gap:16px;align-items:center;flex-wrap:wrap}
.mca-bar nav a{color:#b9c0da;text-decoration:none}
.mca-bar nav a:hover{color:#fff;text-decoration:underline}
.mca-cta{background:#ff7a1a;color:#12132e !important;padding:5px 12px;border-radius:999px;font-weight:600}
.mca-cta:hover{background:#ffb347;text-decoration:none !important}
@media(max-width:520px){.mca-bar-inner{gap:10px}.mca-bar nav{gap:12px;font-size:12px}}
</style>`

// handleSubscribePage is the archive's own signup: GET renders a form, POST adds the
// contact to the workspace's public list through the same service the notification
// centre and blog use, so double opt-in, rate limiting and consent behave exactly as
// they do everywhere else.
func (h *ArchiveHandler) handleSubscribePage(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, settings *domain.ArchiveSettings) {
	if len(settings.SubscribeListIDs) == 0 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.submitSubscribe(w, r, workspace, settings)
		return
	}
	// A successful POST redirects here with ?ok=1 (POST/redirect/GET), so a refresh
	// cannot resubmit and silently double-add.
	h.renderSubscribeForm(w, workspace, settings, r.URL.Query().Get("ok") == "1", r.URL.Query().Get("error"))
}

func (h *ArchiveHandler) submitSubscribe(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, settings *domain.ArchiveSettings) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/archive/"+slugify(workspace.Name)+"/subscribe?error=invalid", http.StatusSeeOther)
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	if email == "" {
		http.Redirect(w, r, "/archive/"+slugify(workspace.Name)+"/subscribe?error=email", http.StatusSeeOther)
		return
	}

	contact := domain.Contact{Email: email}
	if name := strings.TrimSpace(r.PostFormValue("first_name")); name != "" {
		contact.FirstName = &domain.NullableString{String: name}
	}
	if name := strings.TrimSpace(r.PostFormValue("last_name")); name != "" {
		contact.LastName = &domain.NullableString{String: name}
	}
	req := &domain.SubscribeToListsRequest{
		WorkspaceID: workspace.ID,
		Contact:     contact,
		ListIDs:     settings.SubscribeListIDs,
	}
	if h.listService != nil {
		if err := h.listService.SubscribeToLists(r.Context(), req, false); err != nil {
			h.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Archive subscribe failed")
			http.Redirect(w, r, "/archive/"+slugify(workspace.Name)+"/subscribe?error=subscribe", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/archive/"+slugify(workspace.Name)+"/subscribe?ok=1", http.StatusSeeOther)
}

func (h *ArchiveHandler) renderSubscribeForm(w http.ResponseWriter, workspace *domain.Workspace, settings *domain.ArchiveSettings, ok bool, errCode string) {
	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<meta name=\"robots\" content=\"noindex\">")
	b.WriteString("<title>Subscribe · " + html.EscapeString(barName(workspace, settings)) + "</title>")
	b.WriteString(archiveCSS)
	b.WriteString(archiveBarCSS)
	b.WriteString(signupCSS)
	b.WriteString(signupCSS)
	b.WriteString("</head><body>")
	b.WriteString(toolbarHTML(archiveBarOptions{
		workspaceSlug: slugify(workspace.Name),
		workspaceName: barName(workspace, settings),
		subscribeURL:  "/archive/" + slugify(workspace.Name) + "/subscribe",
	}))
	b.WriteString("<main><header><h1>Subscribe to " + html.EscapeString(barName(workspace, settings)) + "</h1>")
	b.WriteString("<p class=\"count\">One email when there is something worth showing you.</p></header>")

	if ok {
		// No promise of a confirmation email: whether one is sent depends on the
		// list's double opt-in setting, which this page does not read. Saying "check
		// your inbox" when nothing is sent is worse than saying nothing.
		b.WriteString(`<p class="note ok">You are on the list.</p>`)
	}
	switch errCode {
	case "email":
		b.WriteString(`<p class="note bad">That email address does not look right.</p>`)
	case "subscribe":
		b.WriteString(`<p class="note bad">That address could not be added just now. Try again in a minute.</p>`)
	case "invalid":
		b.WriteString(`<p class="note bad">The form could not be read. Please try again.</p>`)
	}

	b.WriteString(`<form method="post" action="/archive/` + html.EscapeString(slugify(workspace.Name)) + `/subscribe">`)
	b.WriteString(`<div class="names">`)
	b.WriteString(`<label>First name<input name="first_name" type="text" autocomplete="given-name" placeholder="optional"></label>`)
	b.WriteString(`<label>Last name<input name="last_name" type="text" autocomplete="family-name" placeholder="optional"></label>`)
	b.WriteString(`</div>`)
	b.WriteString(`<label>Email<input name="email" type="email" required autocomplete="email" placeholder="you@example.com"></label>`)
	b.WriteString(`<button type="submit">Subscribe</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`<p class="small"><a href="/archive/` + html.EscapeString(slugify(workspace.Name)) +
		`/">Read past issues instead →</a></p>`)
	b.WriteString(`</main></body></html>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write([]byte(b.String()))
}

// inlineSignupForm is the compact signup the archive home carries, the way
// Mailchimp's archive home does: a reader who arrives here may have been forwarded
// a single campaign and never subscribed, and that is the moment to offer it.
func inlineSignupForm(workspaceSlug string) string {
	return `<form class="inline-signup" method="post" action="/archive/` + html.EscapeString(workspaceSlug) + `/subscribe">` +
		`<label class="sr">Email</label>` +
		`<input name="email" type="email" required placeholder="Your email — get the next one" autocomplete="email">` +
		`<button type="submit">Subscribe</button></form>`
}

const signupCSS = `<style>
.inline-signup{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin:22px 0 6px;max-width:560px}
.inline-signup input{flex:1;min-width:220px;padding:11px 14px;border:1px solid #d3d8e6;border-radius:999px;font-size:14px;background:#fff}
.inline-signup button{background:var(--brand);color:#fff;border:0;border-radius:999px;padding:11px 20px;font-size:14px;font-weight:600;cursor:pointer}
.inline-signup button:hover{background:#35449c}
.sr{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}
.small a{color:var(--brand)}
.note{padding:12px 14px;border-radius:10px;margin:18px 0;font-size:14px}
.note.ok{background:#e8f5ec;color:#1c6b34;border:1px solid #bfe3c9}
.note.bad{background:#fdecec;color:#8c1f1f;border:1px solid #f3c9c9}
form{display:flex;flex-direction:column;gap:14px;max-width:420px;margin:26px 0}
label{display:flex;flex-direction:column;gap:6px;font-size:13px;color:var(--muted);letter-spacing:.02em}
input{padding:12px 14px;border:1px solid #d3d8e6;border-radius:10px;font-size:15px;background:#fff}
input:focus{outline:2px solid var(--brand);outline-offset:1px}
button{background:var(--brand);color:#fff;border:0;border-radius:999px;padding:12px 22px;font-size:15px;font-weight:600;cursor:pointer;align-self:flex-start}
button:hover{background:#35449c}
.small{font-size:13px;color:var(--muted)}
</style>`
