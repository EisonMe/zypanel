package web

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/EisonMe/zypanel/internal/execx"
)

var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,127}$`)

func validRef(ref string) bool { return refPattern.MatchString(ref) }

type containerRow struct {
	ID         string `json:"ID"`
	Names      string `json:"Names"`
	Image      string `json:"Image"`
	State      string `json:"State"`
	Status     string `json:"Status"`
	Ports      string `json:"Ports"`
	RunningFor string `json:"RunningFor"`
}

type containersPage struct {
	BaseData
	Items   []containerRow
	Error   string
	Total   int
	Running int
	Stopped int
}

func (s *Server) listContainers() ([]containerRow, string) {
	res := execx.Run([]string{s.DockerBin, "ps", "-a", "--no-trunc", "--format", "{{json .}}"}, 20*time.Second)
	if !res.OK() {
		return nil, firstLine(res.Stderr, res.Err, "docker ps failed")
	}
	items := make([]containerRow, 0, 16)
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var row containerRow
		if err := json.Unmarshal([]byte(line), &row); err == nil {
			items = append(items, row)
		}
	}
	return items, ""
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	page := containersPage{BaseData: withFlash(s.base(r, "Containers"), r)}
	if !s.DockerEnabled {
		s.Renderer.Render(w, http.StatusOK, "containers", page)
		return
	}
	items, errMsg := s.listContainers()
	page.Items = items
	page.Error = errMsg
	page.Total = len(items)
	for _, it := range items {
		if strings.EqualFold(it.State, "running") {
			page.Running++
		} else {
			page.Stopped++
		}
	}
	s.Renderer.Render(w, http.StatusOK, "containers", page)
}

type containerDetailPage struct {
	BaseData
	Ref      string
	Name     string
	Image    string
	State    string
	Status   string
	Created  string
	Command  string
	Ports    string
	Mounts   string
	Networks string
	Inspect  string
	Logs     string
	Error    string
}

func (s *Server) handleContainerDetail(w http.ResponseWriter, r *http.Request) {
	ref := sanitizeRef(r.PathValue("ref"))
	page := containerDetailPage{BaseData: withFlash(s.base(r, "Container"), r), Ref: ref}
	if !s.DockerEnabled || !validRef(ref) {
		page.Error = "container reference is not valid"
		s.Renderer.Render(w, http.StatusOK, "container", page)
		return
	}

	res := execx.Run([]string{s.DockerBin, "inspect", ref}, 20*time.Second)
	if !res.OK() {
		page.Error = firstLine(res.Stderr, res.Err, "docker inspect failed")
		s.Renderer.Render(w, http.StatusOK, "container", page)
		return
	}

	var parsed []struct {
		Name   string `json:"Name"`
		Config struct {
			Image  string            `json:"Image"`
			Cmd    []string          `json:"Cmd"`
			Env    []string          `json:"Env"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status   string `json:"Status"`
			Running  bool   `json:"Running"`
			ExitCode int    `json:"ExitCode"`
		} `json:"State"`
		Created    string `json:"Created"`
		HostConfig struct {
			NetworkMode string `json:"NetworkMode"`
		} `json:"HostConfig"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIp   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
		Mounts []struct {
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			Mode        string `json:"Mode"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &parsed); err != nil || len(parsed) == 0 {
		page.Error = "could not parse docker inspect output"
		s.Renderer.Render(w, http.StatusOK, "container", page)
		return
	}

	info := parsed[0]
	page.Name = strings.TrimPrefix(info.Name, "/")
	page.Image = info.Config.Image
	page.State = info.State.Status
	page.Status = strings.Join(info.Config.Cmd, " ")
	page.Created = info.Created
	page.Networks = info.HostConfig.NetworkMode

	ports := make([]string, 0, 8)
	for cport, binds := range info.NetworkSettings.Ports {
		if len(binds) == 0 {
			ports = append(ports, cport)
			continue
		}
		for _, b := range binds {
			ports = append(ports, b.HostIp+":"+b.HostPort+" → "+cport)
		}
	}
	page.Ports = strings.Join(ports, "\n")

	mounts := make([]string, 0, 8)
	for _, m := range info.Mounts {
		mode := "ro"
		if m.RW {
			mode = "rw"
		}
		mounts = append(mounts, m.Source+" → "+m.Destination+" ("+mode+")")
	}
	page.Mounts = strings.Join(mounts, "\n")

	pretty, _ := json.MarshalIndent(parsed[0], "", "  ")
	page.Inspect = string(pretty)

	logRes := execx.Run([]string{s.DockerBin, "logs", "--tail", "100", ref}, 20*time.Second)
	page.Logs = logRes.Stdout
	if logRes.Stderr != "" {
		page.Logs += logRes.Stderr
	}

	s.Renderer.Render(w, http.StatusOK, "container", page)
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	ref := sanitizeRef(r.PathValue("ref"))
	if !s.DockerEnabled || !validRef(ref) {
		http.Error(w, "invalid container reference", http.StatusBadRequest)
		return
	}
	tail := "200"
	if v := r.URL.Query().Get("tail"); v != "" {
		filtered := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, v)
		if filtered != "" && len(filtered) <= 5 {
			tail = filtered
		}
	}
	res := execx.Run([]string{s.DockerBin, "logs", "--tail", tail, ref}, 30*time.Second)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(res.Stdout + res.Stderr))
}

func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	ref := sanitizeRef(r.PathValue("ref"))
	action := strings.TrimSpace(r.FormValue("action"))

	allowed := map[string]bool{"start": true, "stop": true, "restart": true}
	if !s.DockerEnabled || s.ReadOnly || !validRef(ref) || !allowed[action] {
		s.record(r, "docker."+action, ref, false, "rejected")
		http.Redirect(w, r, "/containers?err=action-rejected", http.StatusSeeOther)
		return
	}

	res := execx.Run([]string{s.DockerBin, action, ref}, 120*time.Second)
	s.record(r, "docker."+action, ref, res.OK(), firstLine(res.Stderr, res.Err))
	if res.OK() {
		http.Redirect(w, r, "/containers/"+ref+"?ok=action-done", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/containers/"+ref+"?err=docker-"+"action-failed", http.StatusSeeOther)
}

func (s *Server) handleContainerDelete(w http.ResponseWriter, r *http.Request) {
	ref := sanitizeRef(r.PathValue("ref"))
	confirm := strings.TrimSpace(r.FormValue("confirm"))
	force := r.FormValue("force") != ""

	if !s.DockerEnabled || s.ReadOnly || !validRef(ref) || confirm != "DELETE" {
		s.record(r, "docker.remove", ref, false, "confirmation missing")
		http.Redirect(w, r, "/containers/"+ref+"?err=confirm-required", http.StatusSeeOther)
		return
	}

	argv := []string{s.DockerBin, "rm", ref}
	if force {
		argv = []string{s.DockerBin, "rm", "-f", ref}
	}
	res := execx.Run(argv, 120*time.Second)
	s.record(r, "docker.remove", ref, res.OK(), firstLine(res.Stderr, res.Err))
	if res.OK() {
		http.Redirect(w, r, "/containers?ok=removed", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/containers/"+ref+"?err=remove-failed", http.StatusSeeOther)
}

func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	confirm := strings.TrimSpace(r.FormValue("confirm"))
	if !s.DockerEnabled || s.ReadOnly || confirm != "PRUNE" {
		s.record(r, "docker.prune", "system", false, "confirmation missing")
		http.Redirect(w, r, "/containers?err=confirm-required", http.StatusSeeOther)
		return
	}
	res := execx.Run([]string{s.DockerBin, "system", "prune", "-f"}, 180*time.Second)
	s.record(r, "docker.prune", "system", res.OK(), firstLine(res.Stderr, res.Err))
	if res.OK() {
		http.Redirect(w, r, "/containers?ok=pruned", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/containers?err=prune-failed", http.StatusSeeOther)
}
