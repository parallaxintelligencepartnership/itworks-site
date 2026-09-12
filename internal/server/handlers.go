package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"itworks.dev/internal/badge"
	"itworks.dev/internal/store"
	"itworks.dev/web"
)

const maxBodyBytes = 8192

// landingPreviewCount is how many approved entries the landing page shows in
// its wall preview.
const landingPreviewCount = 3

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	approved, err := s.db.ListApproved()
	if err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC()
	green, amber, red := sampleBadges(now)

	views := s.toEntryViews(approved, now)
	if len(views) > landingPreviewCount {
		views = views[:landingPreviewCount]
	}

	var data LandingData
	data.Title = "itworks.dev"
	data.EntryCount = len(approved)
	data.Badges.Green, data.Badges.Amber, data.Badges.Red = green, amber, red
	data.InstallCommands = InstallCommands
	data.Entries = views

	s.render(w, "landing.html", data)
}

func (s *Server) handleWall(w http.ResponseWriter, r *http.Request) {
	approved, err := s.db.ListApproved()
	if err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC()
	data := WallData{Title: "The wall", Entries: s.toEntryViews(approved, now)}
	s.render(w, "wall.html", data)
}

func (s *Server) handleEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := s.db.GetByID(id)
	if err != nil || e.Status != store.StatusApproved {
		http.NotFound(w, r)
		return
	}
	now := time.Now().UTC()
	data := EntryData{Title: e.Name, Entry: s.toEntryView(e, now)}
	s.render(w, "entry.html", data)
}

func (s *Server) handleAPIList(w http.ResponseWriter, r *http.Request) {
	approved, err := s.db.ListApproved()
	if err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC()
	s.writeJSON(w, http.StatusOK, map[string]any{"entries": s.toEntryViews(approved, now)})
}

func (s *Server) handleAPICreate(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)

	// The rate limiter is checked and consumed before anything about the
	// request body is read, so malformed, oversized, and wrong content type
	// requests all count against the caller's quota.
	allowed, retryAfter := s.limiter.Allow(ip, time.Now())
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
		s.finishPost(w, ip, http.StatusTooManyRequests, "too many submissions from this address, try again later")
		return
	}

	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		s.finishPost(w, ip, http.StatusUnsupportedMediaType, "content type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	req, err := decodeSubmitRequest(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			s.finishPost(w, ip, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		s.finishPost(w, ip, http.StatusBadRequest, "the request body must be valid JSON containing only the documented fields")
		return
	}

	ne, msg := validateSubmit(req, time.Now())
	if msg != "" {
		s.finishPost(w, ip, http.StatusBadRequest, msg)
		return
	}

	pendingCount, err := s.db.CountPending()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if pendingCount >= s.pendingCap {
		s.finishPost(w, ip, http.StatusServiceUnavailable, "submissions are paused, try again later")
		return
	}

	id, err := s.db.Create(ne)
	if err != nil {
		s.serverError(w, err)
		return
	}

	s.logger.Printf("POST /api/entries ip=%s status=%d", ip, http.StatusCreated)
	s.writeJSON(w, http.StatusCreated, map[string]any{
		"id":        id,
		"badge_url": s.badgeURL(id),
		"entry_url": s.entryURL(id),
		"status":    store.StatusPending,
	})
}

func (s *Server) finishPost(w http.ResponseWriter, ip string, status int, msg string) {
	s.logger.Printf("POST /api/entries ip=%s status=%d", ip, status)
	s.writeJSONError(w, status, msg)
}

func (s *Server) handleBadge(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("idsvg")
	id, ok := strings.CutSuffix(name, ".svg")
	if !ok || id == "" {
		http.NotFound(w, r)
		return
	}

	e, err := s.db.GetByID(id)
	if err != nil || e.Status == store.StatusHidden {
		http.NotFound(w, r)
		return
	}

	auditDate, _ := time.Parse("2006-01-02", e.AuditDate)
	color := badge.Color(auditDate, e.CriticalOpen, time.Now())
	svg := badge.Render(e.AuditDate, e.CriticalOpen, color)

	h := w.Header()
	h.Set("Content-Security-Policy", cspBadge)
	h.Set("Content-Type", "image/svg+xml; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write(svg)
}

func (s *Server) handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/css; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write(s.staticCSS)
}

// handleStaticFont serves one embedded self hosted font file or its license
// text. Only plain file names inside web/static/fonts are reachable.
func (s *Server) handleStaticFont(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	var ctype string
	switch {
	case strings.HasSuffix(name, ".woff2"):
		ctype = "font/woff2"
	case strings.HasSuffix(name, ".txt"):
		ctype = "text/plain; charset=utf-8"
	default:
		http.NotFound(w, r)
		return
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}

	body, err := web.FS.ReadFile("static/fonts/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleAdminList(w http.ResponseWriter, r *http.Request) {
	pending, err := s.db.ListPending()
	if err != nil {
		s.serverError(w, err)
		return
	}
	approved, err := s.db.ListApproved()
	if err != nil {
		s.serverError(w, err)
		return
	}
	hidden, err := s.db.ListHidden()
	if err != nil {
		s.serverError(w, err)
		return
	}

	now := time.Now().UTC()
	data := AdminData{
		Title:    "Admin",
		Pending:  s.toEntryViews(pending, now),
		Approved: s.toEntryViews(approved, now),
		Hidden:   s.toEntryViews(hidden, now),
	}
	s.render(w, "admin.html", data)
}

func (s *Server) handleAdminApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.db.Approve(id)
	s.logAdmin(r, "approve", id, err)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleAdminHide(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.db.Hide(id)
	s.logAdmin(r, "hide", id, err)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) logAdmin(r *http.Request, action, id string, err error) {
	result := "ok"
	if err != nil {
		result = err.Error()
	}
	s.logger.Printf("admin action=%s id=%s user=%s result=%s", action, id, r.Header.Get(adminUserHeader), result)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeJSONError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.logger.Printf("server error: %v", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
