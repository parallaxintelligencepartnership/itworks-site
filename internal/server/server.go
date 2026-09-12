// Package server implements the itworks.dev HTTP surface: public pages, the
// JSON submit and list API, SVG badges, and the admin approval screen.
package server

import (
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"itworks.dev/internal/store"
	"itworks.dev/web"
)

const (
	defaultPendingCap = 500
	rateLimitPerHour  = 5
	adminUserHeader   = "X-Authentik-Username"
	cspDefault        = "default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self' data:; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"
	cspBadge          = "default-src 'none'"
)

// Config configures a Server.
type Config struct {
	BaseURL    string
	TrustProxy bool
	PendingCap int // 0 means defaultPendingCap
}

// Server holds everything the HTTP handlers need.
type Server struct {
	db         *store.DB
	tmpl       *template.Template
	baseURL    string
	trustProxy bool
	pendingCap int
	limiter    *ipLimiter
	staticCSS  []byte
	logger     *log.Logger
}

// New builds a Server from an open store and config, parsing the embedded
// templates and loading the embedded stylesheet.
func New(db *store.DB, cfg Config) (*Server, error) {
	tmpl, err := template.ParseFS(web.FS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	css, err := web.FS.ReadFile("static/site.css")
	if err != nil {
		return nil, err
	}

	pendingCap := cfg.PendingCap
	if pendingCap <= 0 {
		pendingCap = defaultPendingCap
	}

	return &Server{
		db:         db,
		tmpl:       tmpl,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		trustProxy: cfg.TrustProxy,
		pendingCap: pendingCap,
		limiter:    newIPLimiter(rateLimitPerHour, time.Hour),
		staticCSS:  css,
		logger:     log.Default(),
	}, nil
}

// Routes returns the fully wired handler for the service.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleLanding)
	mux.HandleFunc("GET /wall", s.handleWall)
	mux.HandleFunc("GET /e/{id}", s.handleEntry)
	mux.HandleFunc("GET /api/entries", s.handleAPIList)
	mux.HandleFunc("POST /api/entries", s.handleAPICreate)
	mux.HandleFunc("GET /badge/{idsvg}", s.handleBadge)
	mux.HandleFunc("GET /static/site.css", s.handleStaticCSS)
	mux.HandleFunc("GET /static/fonts/{file}", s.handleStaticFont)
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	mux.HandleFunc("GET /admin", s.requireAdmin(s.handleAdminList))
	mux.HandleFunc("POST /admin/entries/{id}/approve", s.requireAdmin(s.handleAdminApprove))
	mux.HandleFunc("POST /admin/entries/{id}/hide", s.requireAdmin(s.handleAdminHide))

	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", cspDefault)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// requireAdmin guards /admin routes: the forward auth username header must
// be present and non empty, and any POST must carry an Origin header whose
// host matches the request Host. Traefik forward auth is the primary guard;
// this is defense in depth.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get(adminUserHeader)
		if user == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			origin := r.Header.Get("Origin")
			if origin == "" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

// clientIP returns the caller's IP per the trust proxy setting. It is used
// only for rate limiting and access logs, never persisted to the database.
func (s *Server) clientIP(r *http.Request) string {
	if s.trustProxy {
		if ip := r.Header.Get("X-Real-Ip"); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
