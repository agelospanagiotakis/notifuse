package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ArchiveAdminHandler is the console side of the campaign archive: the settings that
// were previously SQL, and the list of archived campaigns with the ability to
// unpublish one. The public reader is ArchiveHandler; this is the operator.
type ArchiveAdminHandler struct {
	workspaceRepo domain.WorkspaceRepository
	archiveRepo   domain.CampaignArchiveRepository
	templateRepo  domain.TemplateRepository
	logger        logger.Logger
	getJWTSecret  func() ([]byte, error)
}

func NewArchiveAdminHandler(
	workspaceRepo domain.WorkspaceRepository,
	archiveRepo domain.CampaignArchiveRepository,
	templateRepo domain.TemplateRepository,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *ArchiveAdminHandler {
	return &ArchiveAdminHandler{
		workspaceRepo: workspaceRepo,
		archiveRepo:   archiveRepo,
		templateRepo:  templateRepo,
		logger:        logger,
		getJWTSecret:  getJWTSecret,
	}
}

type archiveEntryRow struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Subject   string `json:"subject"`
	SentAt    string `json:"sent_at"`
	Template  string `json:"template_id"`
	PreviewAt string `json:"preview_url"`
}

func (h *ArchiveAdminHandler) RegisterRoutes(mux *http.ServeMux) {
	requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	mux.Handle("/api/archive.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/archive.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/archive.remove", requireAuth(http.HandlerFunc(h.handleRemove)))
	mux.Handle("/api/archive.publish", requireAuth(http.HandlerFunc(h.handlePublish)))
	mux.Handle("/api/archive.setAudienceListed", requireAuth(http.HandlerFunc(h.handleSetAudienceListed)))
	// The Lists screen needs the mapping from a platform list to its archive page, and nothing
	// else. archive.get would hand it 182 entries to answer that.
	mux.Handle("/api/archive.listAudiences", requireAuth(http.HandlerFunc(h.handleListAudiences)))
}

func (h *ArchiveAdminHandler) workspace(w http.ResponseWriter, r *http.Request) *domain.Workspace {
	id := r.URL.Query().Get("workspace_id")
	if id == "" && r.Method == http.MethodPost {
		var body struct {
			WorkspaceID string `json:"workspace_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id = body.WorkspaceID
	}
	if id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return nil
	}
	ws, err := h.workspaceRepo.GetByID(r.Context(), id)
	if err != nil || ws == nil {
		WriteJSONError(w, "workspace not found", http.StatusNotFound)
		return nil
	}
	return ws
}

// handleGet returns the current settings and the archived campaigns in one call: the
// screen needs both, and two round trips would let them disagree.
func (h *ArchiveAdminHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	ws := h.workspace(w, r)
	if ws == nil {
		return
	}

	settings := domain.ArchiveSettings{}
	if ws.Settings.Archive != nil {
		settings = *ws.Settings.Archive
	}
	// Never send the password to the browser. The screen shows whether a gate is
	// set, and writes a new password only when the operator types one.
	gateSet := settings.BasicAuthUser != ""
	settings.BasicAuthPassword = ""

	entries := []archiveEntryRow{}
	if err := h.archiveRepo.EnsureSchema(r.Context(), ws.ID); err == nil {
		list, err := h.archiveRepo.List(r.Context(), ws.ID)
		if err == nil {
			sort.SliceStable(list, func(i, j int) bool { return list[i].SentAt.After(list[j].SentAt) })
			for _, e := range list {
				entries = append(entries, archiveEntryRow{
					Slug:      e.Slug,
					Title:     e.Title,
					Subject:   e.Subject,
					SentAt:    e.SentAt.UTC().Format("2006-01-02"),
					Template:  e.TemplateID,
					PreviewAt: "/archive/" + slugify(ws.Name) + "/" + e.Slug,
				})
			}
		}
	}

	// Audiences, with the URL each one's page lives at. This is the console's answer to
	// "where is this list's archive?" — the question the public pages deliberately do not
	// advertise. A list with no registry row is reported by its id, which is also the URL it
	// resolves at, so the link is never dead.
	audiences := []map[string]any{}
	if err := h.archiveRepo.EnsureSchema(r.Context(), ws.ID); err == nil {
		if list, err := h.archiveRepo.ListAudiences(r.Context(), ws.ID); err == nil {
			for _, a := range list {
				slug := a.Slug
				if slug == "" {
					slug = a.ListID
				}
				name := a.Name
				if name == "" {
					name = a.ListID
				}
				audiences = append(audiences, map[string]any{
					"list_id":          a.ListID,
					"notifuse_list_id": a.NotifuseListID,
					"slug":             slug,
					"name":             name,
					"listed":           a.Listed,
					"campaigns":        a.Campaigns,
					"url":              "/archive/" + slugify(ws.Name) + "/" + slug + "/",
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"settings":   settings,
		"gate_set":   gateSet,
		"entries":    entries,
		"audiences":  audiences,
		"public_url": "/archive/" + slugify(ws.Name) + "/",
	})
}

// handleUpdate writes the settings. The password is only replaced when a non-empty
// value arrives, so saving the screen does not silently unlock the archive.
func (h *ArchiveAdminHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Settings    struct {
			Enabled           bool     `json:"enabled"`
			Title             string   `json:"title"`
			SubscribeURL      string   `json:"subscribe_url"`
			SubscribeListIDs  []string `json:"subscribe_list_ids"`
			BasicAuthUser     string   `json:"basic_auth_user"`
			BasicAuthPassword string   `json:"basic_auth_password"`
			ClearGate         bool     `json:"clear_gate"`
			ShowAudienceNav   bool     `json:"show_audience_nav"`
		} `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ws, err := h.workspaceRepo.GetByID(r.Context(), body.WorkspaceID)
	if err != nil || ws == nil {
		WriteJSONError(w, "workspace not found", http.StatusNotFound)
		return
	}

	next := domain.ArchiveSettings{
		Enabled:          body.Settings.Enabled,
		Title:            strings.TrimSpace(body.Settings.Title),
		SubscribeURL:     strings.TrimSpace(body.Settings.SubscribeURL),
		SubscribeListIDs: body.Settings.SubscribeListIDs,
		BasicAuthUser:    strings.TrimSpace(body.Settings.BasicAuthUser),
		ShowAudienceNav:  body.Settings.ShowAudienceNav,
	}
	if body.Settings.ClearGate {
		next.BasicAuthUser = ""
	} else if ws.Settings.Archive != nil {
		// Preserve the stored password unless a new one was typed.
		next.BasicAuthPassword = ws.Settings.Archive.BasicAuthPassword
	}
	if body.Settings.BasicAuthPassword != "" {
		next.BasicAuthPassword = body.Settings.BasicAuthPassword
	}

	ws.Settings.Archive = &next
	if err := h.workspaceRepo.Update(r.Context(), ws); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to save archive settings")
		WriteJSONError(w, "could not save settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleRemove unpublishes one campaign. It deletes the archive row only: the
// template stays, so nothing that was sent is lost — the campaign just stops being
// offered publicly, which is exactly what an operator asking to remove it means.
func (h *ArchiveAdminHandler) handleRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Slug        string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Slug == "" {
		WriteJSONError(w, "slug is required", http.StatusBadRequest)
		return
	}
	if err := h.archiveRepo.Remove(r.Context(), body.WorkspaceID, body.Slug); err != nil {
		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handlePublish adds a template to the archive, or updates the entry it already has.
func (h *ArchiveAdminHandler) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		TemplateID  string `json:"template_id"`
		Title       string `json:"title"`
		SentAt      string `json:"sent_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TemplateID == "" {
		WriteJSONError(w, "template_id is required", http.StatusBadRequest)
		return
	}

	tpl, err := h.templateRepo.GetTemplateByID(r.Context(), body.WorkspaceID, body.TemplateID, 0)
	if err != nil || tpl == nil {
		WriteJSONError(w, "template not found", http.StatusNotFound)
		return
	}

	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = tpl.Name
	}
	sentAt := strings.TrimSpace(body.SentAt)
	if sentAt == "" {
		sentAt = tpl.CreatedAt.UTC().Format("2006-01-02")
	}
	parsed, err := parseArchiveDate(sentAt)
	if err != nil {
		WriteJSONError(w, "sent_at must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	entry := &domain.ArchiveEntry{
		Slug:            archiveSlug(parsed, title, tpl.ID),
		TemplateID:      tpl.ID,
		TemplateVersion: tpl.Version,
		Title:           title,
		SentAt:          parsed,
	}
	if err := h.archiveRepo.Upsert(r.Context(), body.WorkspaceID, entry); err != nil {
		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "slug": entry.Slug})
}

func parseArchiveDate(value string) (time.Time, error) {
	return time.Parse("2006-01-02", value)
}

// archiveSlug mirrors the importer's scheme: date, then what the title transliterates
// to, then the id when the title carries no latin characters at all (Greek, mostly).
func archiveSlug(sentAt time.Time, title, templateID string) string {
	date := sentAt.UTC().Format("2006-01-02")
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 60 {
		slug = strings.Trim(slug[:60], "-")
	}
	if slug == "" {
		return date + "-" + strings.TrimPrefix(templateID, "mc_")[:6]
	}
	return date + "-" + slug
}

// handleSetAudienceListed ticks or unticks one audience in the public archive.
//
// A dedicated endpoint rather than a field on archive.update: this is a per-row decision made
// from a checkbox, and routing it through the settings form would mean a save that rewrites
// the whole settings blob to change one boolean.
func (h *ArchiveAdminHandler) handleSetAudienceListed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		ListID      string `json:"list_id"`
		Listed      bool   `json:"listed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.WorkspaceID == "" || body.ListID == "" {
		WriteJSONError(w, "workspace_id and list_id are required", http.StatusBadRequest)
		return
	}

	ws, err := h.workspaceRepo.GetByID(r.Context(), body.WorkspaceID)
	if err != nil || ws == nil {
		WriteJSONError(w, "workspace not found", http.StatusNotFound)
		return
	}
	if err := h.archiveRepo.EnsureSchema(r.Context(), ws.ID); err != nil {
		WriteJSONError(w, "archive unavailable", http.StatusInternalServerError)
		return
	}
	if err := h.archiveRepo.SetAudienceListed(r.Context(), ws.ID, body.ListID, body.Listed); err != nil {
		if errors.Is(err, domain.ErrArchiveAudienceNotFound) {
			WriteJSONError(w, "audience not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("workspace_id", ws.ID).WithField("list_id", body.ListID).
			WithField("error", err.Error()).Error("Failed to set audience listing")
		WriteJSONError(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleListAudiences returns the audience registry alone: id, Notifuse list, slug, name,
// listing state and public URL. Used by the Lists screen to link each list to its archive.
func (h *ArchiveAdminHandler) handleListAudiences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	ws, err := h.workspaceRepo.GetByID(r.Context(), workspaceID)
	if err != nil || ws == nil {
		WriteJSONError(w, "workspace not found", http.StatusNotFound)
		return
	}
	audiences := []map[string]any{}
	if err := h.archiveRepo.EnsureSchema(r.Context(), ws.ID); err == nil {
		if list, err := h.archiveRepo.ListAudiences(r.Context(), ws.ID); err == nil {
			for _, a := range list {
				slug := a.Slug
				if slug == "" {
					slug = a.ListID
				}
				audiences = append(audiences, map[string]any{
					"list_id":          a.ListID,
					"notifuse_list_id": a.NotifuseListID,
					"slug":             slug,
					"name":             nameOr(a.Name, a.ListID),
					"listed":           a.Listed,
					"campaigns":        a.Campaigns,
					"url":              "/archive/" + slugify(ws.Name) + "/" + slug + "/",
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"audiences": audiences})
}

// nameOr keeps the caller readable: an audience without a registry row is named by its id.
func nameOr(name, fallback string) string {
	if strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}
