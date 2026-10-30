package web

import (
	"net/http"
	"strings"

	"github.com/EisonMe/zypanel/internal/auth"
)

// Handler builds the complete HTTP router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"` + s.Version + `"}`))
	})
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLoginPost)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.Handle("GET /static/", http.StripPrefix("/static/", StaticHandler()))

	// Authenticated routes.
	protected := http.NewServeMux()
	protected.HandleFunc("GET /{$}", s.handleDashboard)

	protected.HandleFunc("GET /containers", s.handleContainers)
	protected.HandleFunc("POST /containers/prune", s.RequireCSRF(s.handleContainerPrune))
	protected.HandleFunc("GET /containers/{ref}", s.handleContainerDetail)
	protected.HandleFunc("GET /containers/{ref}/logs", s.handleContainerLogs)
	protected.HandleFunc("POST /containers/{ref}/action", s.RequireCSRF(s.handleContainerAction))
	protected.HandleFunc("POST /containers/{ref}/delete", s.RequireCSRF(s.handleContainerDelete))

	protected.HandleFunc("GET /services", s.handleServices)
	protected.HandleFunc("POST /services/action", s.RequireCSRF(s.handleServiceAction))

	protected.HandleFunc("GET /cloudflared", s.handleCloudflared)
	protected.HandleFunc("POST /cloudflared/add", s.RequireCSRF(s.handleCloudflaredAdd))
	protected.HandleFunc("POST /cloudflared/remove", s.RequireCSRF(s.handleCloudflaredRemove))
	protected.HandleFunc("POST /cloudflared/restart", s.RequireCSRF(s.handleCloudflaredRestart))

	protected.HandleFunc("GET /openclaw", s.handleOpenClaw)
	protected.HandleFunc("GET /ports", s.handlePorts)
	protected.HandleFunc("GET /audit", s.handleAudit)
	protected.HandleFunc("GET /settings", s.handleSettings)

	root := http.NewServeMux()
	root.Handle("/", s.RequireAuth(protected))

	// Merge: public first, then the authenticated catch-all.
	combined := http.NewServeMux()
	combined.Handle("/healthz", mux)
	combined.Handle("/login", mux)
	combined.Handle("/static/", mux)
	combined.Handle("/", root)

	return LogRequests(SecurityHeaders(s.WithCSRF(combined)))
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if _, ok := s.Auth.Validate(c.Value); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	s.Renderer.Render(w, http.StatusOK, "login", struct {
		BaseData
		Error string
	}{BaseData: s.base(r, "Sign in"), Error: r.URL.Query().Get("err")})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?err=bad-request", http.StatusSeeOther)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	if !s.Auth.Authenticate(username, password) {
		http.Redirect(w, r, "/login?err=invalid-credentials", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.Auth.NewSession(),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.Auth.TTL().Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// flash picks a message from the query string.
func flash(r *http.Request) (string, string) {
	if v := r.URL.Query().Get("ok"); v != "" {
		return v, "ok"
	}
	if v := r.URL.Query().Get("err"); v != "" {
		return v, "err"
	}
	return "", ""
}

func withFlash(b BaseData, r *http.Request) BaseData {
	b.Flash, b.FlashKind = flash(r)
	return b
}

func sanitizeRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if len(ref) > 128 {
		ref = ref[:128]
	}
	return ref
}

// newCSRF exposes a token for forms rendered outside the context helper.
func (s *Server) newCSRF() string { return auth.NewCSRFToken() }
