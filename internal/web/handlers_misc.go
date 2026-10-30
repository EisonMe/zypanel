package web

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/EisonMe/zypanel/internal/execx"
)

// ---------------------------------------------------------------- openclaw ---

type openClawPage struct {
	BaseData
	Available bool
	Status    string
	Agents    string
	Error     string
}

func (s *Server) handleOpenClaw(w http.ResponseWriter, r *http.Request) {
	page := openClawPage{BaseData: withFlash(s.base(r, "OpenClaw"), r)}
	if !s.OpenClawEnabled {
		s.Renderer.Render(w, http.StatusOK, "openclaw", page)
		return
	}
	if !execx.LookPath(s.OpenClawBin) {
		page.Error = "容器内未找到 openclaw 可执行文件"
		s.Renderer.Render(w, http.StatusOK, "openclaw", page)
		return
	}

	page.Available = true
	if res := execx.Run([]string{s.OpenClawBin, "status"}, 30*time.Second); res.OK() {
		page.Status = strings.TrimSpace(res.Stdout)
	} else {
		page.Error = firstLine(res.Stderr, res.Err, "openclaw status failed")
	}
	if res := execx.Run([]string{s.OpenClawBin, "agents", "list"}, 30*time.Second); res.OK() {
		page.Agents = strings.TrimSpace(res.Stdout)
	}
	s.Renderer.Render(w, http.StatusOK, "openclaw", page)
}

// ------------------------------------------------------------------ ports ---

type portRow struct {
	Proto   string
	Address string
	Port    string
	Process string
}

type portsPage struct {
	BaseData
	Items []portRow
	Error string
}

func (s *Server) handlePorts(w http.ResponseWriter, r *http.Request) {
	page := portsPage{BaseData: withFlash(s.base(r, "Ports"), r)}
	res := execx.Run([]string{"ss", "-ltnp"}, 20*time.Second)

	if !res.OK() {
		// Fall back to /proc/net/tcp when ss is unavailable.
		page.Error = "ss 不可用，仅显示 /proc/net/tcp"
	}

	for _, line := range strings.Split(res.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "LISTEN") {
			continue
		}
		local := fields[3]
		idx := strings.LastIndex(local, ":")
		if idx < 0 {
			continue
		}
		addr := local[:idx]
		port := local[idx+1:]
		proc := ""
		if len(fields) >= 6 {
			proc = fields[5]
		}
		page.Items = append(page.Items, portRow{
			Proto:   "tcp",
			Address: addr,
			Port:    port,
			Process: proc,
		})
	}

	s.Renderer.Render(w, http.StatusOK, "ports", page)
}

// ------------------------------------------------------------------ audit ---

type auditPage struct {
	BaseData
	Entries []auditRow
	Error   string
}

type auditRow struct {
	TS     string
	User   string
	IP     string
	Action string
	Target string
	OK     bool
	Detail string
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	page := auditPage{BaseData: withFlash(s.base(r, "Audit log"), r)}
	if s.Audit == nil {
		page.Error = "审计日志未启用"
		s.Renderer.Render(w, http.StatusOK, "audit", page)
		return
	}
	entries, err := s.Audit.Tail(300)
	if err != nil {
		page.Error = err.Error()
		s.Renderer.Render(w, http.StatusOK, "audit", page)
		return
	}
	// Newest first.
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		page.Entries = append(page.Entries, auditRow{
			TS:     e.TS,
			User:   e.User,
			IP:     e.IP,
			Action: e.Action,
			Target: e.Target,
			OK:     e.OK,
			Detail: e.Detail,
		})
	}
	s.Renderer.Render(w, http.StatusOK, "audit", page)
}

// --------------------------------------------------------------- settings ---

type settingsPage struct {
	BaseData
	Addr              string
	DataDir           string
	AuditLog          string
	CookieSecure      bool
	DockerBin         string
	SystemctlBin      string
	CloudflaredBin    string
	CloudflaredConfig string
	OpenClawBin       string
	Hostname          string
	GoVersion         string
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	s.Renderer.Render(w, http.StatusOK, "settings", settingsPage{
		BaseData:          withFlash(s.base(r, "Settings"), r),
		Addr:              s.Addr,
		DataDir:           s.DataDir,
		AuditLog:          s.AuditLog,
		CookieSecure:      s.CookieSecure,
		DockerBin:         s.DockerBin,
		SystemctlBin:      s.SystemctlBin,
		CloudflaredBin:    s.CloudflaredBin,
		CloudflaredConfig: s.CloudflaredConfig,
		OpenClawBin:       s.OpenClawBin,
		Hostname:          hostname,
		GoVersion:         s.GoVersion,
	})
}
