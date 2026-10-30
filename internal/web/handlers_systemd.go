package web

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/EisonMe/zypanel/internal/execx"
)

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9:_.@-]{1,128}\.service$`)

type serviceRow struct {
	Unit        string
	Load        string
	Active      string
	Sub         string
	Description string
	Enabled     string
}

type servicesPage struct {
	BaseData
	Items []serviceRow
	Error string
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	page := servicesPage{BaseData: withFlash(s.base(r, "Services"), r)}
	if !s.SystemdEnabled {
		s.Renderer.Render(w, http.StatusOK, "services", page)
		return
	}

	res := execx.Run([]string{s.SystemctlBin, "list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain"}, 20*time.Second)
	if !res.OK() {
		page.Error = firstLine(res.Stderr, res.Err, "systemctl failed")
		s.Renderer.Render(w, http.StatusOK, "services", page)
		return
	}

	enabled := map[string]string{}
	if fres := execx.Run([]string{s.SystemctlBin, "list-unit-files", "--type=service", "--no-legend", "--no-pager"}, 20*time.Second); fres.OK() {
		for _, line := range strings.Split(fres.Stdout, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				enabled[fields[0]] = fields[1]
			}
		}
	}

	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		unit := strings.TrimPrefix(fields[0], "●")
		if !strings.HasSuffix(unit, ".service") {
			continue
		}
		desc := ""
		if len(fields) > 4 {
			desc = strings.Join(fields[4:], " ")
		}
		page.Items = append(page.Items, serviceRow{
			Unit:        unit,
			Load:        fields[1],
			Active:      fields[2],
			Sub:         fields[3],
			Description: desc,
			Enabled:     enabled[unit],
		})
	}

	s.Renderer.Render(w, http.StatusOK, "services", page)
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	unit := strings.TrimSpace(r.FormValue("unit"))
	action := strings.TrimSpace(r.FormValue("action"))

	allowed := map[string]bool{
		"start": true, "stop": true, "restart": true, "enable": true, "disable": true,
	}
	if !s.SystemdEnabled || s.ReadOnly || !unitPattern.MatchString(unit) || !allowed[action] {
		s.record(r, "systemd."+action, unit, false, "rejected")
		http.Redirect(w, r, "/services?err=action-rejected", http.StatusSeeOther)
		return
	}

	res := execx.Run([]string{s.SystemctlBin, action, unit}, 90*time.Second)
	s.record(r, "systemd."+action, unit, res.OK(), firstLine(res.Stderr, res.Err))
	if res.OK() {
		http.Redirect(w, r, "/services?ok=action-done", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/services?err=action-failed", http.StatusSeeOther)
}
