package web

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/EisonMe/zypanel/internal/audit"
	"github.com/EisonMe/zypanel/internal/auth"
)

type ctxKey string

const (
	ctxUser       ctxKey = "user"
	ctxCSRF       ctxKey = "csrf"
	csrfCookie           = "zypanel_csrf"
	sessionCookie        = "zypanel_session"
)

// Server wires configuration, templates and the audit trail together.
type Server struct {
	Renderer *Renderer
	Auth     *auth.Manager

	DockerEnabled      bool
	SystemdEnabled     bool
	CloudflaredEnabled bool
	OpenClawEnabled    bool
	ReadOnly           bool

	DockerBin         string
	SystemctlBin      string
	CloudflaredBin    string
	CloudflaredConfig string
	OpenClawBin       string

	Audit   *audit.Logger
	Version string
}

// record writes one audit entry, ignoring storage errors.
func (s *Server) record(r *http.Request, action, target string, ok bool, detail string) {
	if s.Audit == nil {
		return
	}
	user, _ := r.Context().Value(ctxUser).(string)
	if user == "" {
		user = "anonymous"
	}
	_ = s.Audit.Write(audit.Entry{
		User:   user,
		IP:     clientIP(r),
		Action: action,
		Target: target,
		OK:     ok,
		Detail: detail,
	})
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		parts := strings.Split(v, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) base(r *http.Request, title string) BaseData {
	user, _ := r.Context().Value(ctxUser).(string)
	csrf, _ := r.Context().Value(ctxCSRF).(string)
	return BaseData{
		Title:    title,
		User:     user,
		CSRF:     csrf,
		Version:  s.Version,
		Path:     r.URL.Path,
		Docker:   s.DockerEnabled,
		Systemd:  s.SystemdEnabled,
		CF:       s.CloudflaredEnabled,
		OpenClaw: s.OpenClawEnabled,
		ReadOnly: s.ReadOnly,
	}
}

// SecurityHeaders adds conservative defaults to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; script-src 'self'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// WithCSRF issues a CSRF cookie when missing and stores the token in context.
func (s *Server) WithCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(csrfCookie); err == nil && c.Value != "" {
			token = c.Value
		}
		if token == "" {
			token = auth.NewCSRFToken()
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookie,
				Value:    token,
				Path:     "/",
				HttpOnly: false,
				Secure:   s.cookieSecure(r),
				SameSite: http.SameSiteLaxMode,
				MaxAge:   30 * 24 * 60 * 60,
			})
		}
		ctx := context.WithValue(r.Context(), ctxCSRF, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) cookieSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// RequireAuth redirects unauthenticated visitors to the login page.
func (s *Server) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		user, ok := s.Auth.Validate(c.Value)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireCSRF rejects state-changing requests without a matching token.
func (s *Server) RequireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			c, err := r.Cookie(csrfCookie)
			form := r.FormValue("csrf")
			if err != nil || !auth.CSRFValid(c.Value, form) {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

// LogRequests prints a compact access log line per request.
func LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/static/") {
			return
		}
		println(r.Method, r.URL.Path, clientIP(r), time.Since(start).String())
	})
}
