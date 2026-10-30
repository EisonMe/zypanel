package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/EisonMe/zypanel/internal/execx"
)

var hostnamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

type cfRule struct {
	Hostname string
	Service  string
}

type cfPage struct {
	BaseData
	ConfigPath string
	Tunnel     string
	Rules      []cfRule
	RawConfig  string
	Error      string
	Available  bool
}

func (s *Server) readConfig() (string, error) {
	data, err := os.ReadFile(s.CloudflaredConfig)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseRuleBlocks splits an ingress list into blocks and extracts hostname/service.
func parseRuleBlocks(text string) []cfRule {
	lines := strings.Split(text, "\n")
	rules := make([]cfRule, 0, 8)
	current := cfRule{}
	inBlock := false

	flush := func() {
		if inBlock {
			rules = append(rules, current)
			current = cfRule{}
			inBlock = false
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			flush()
			inBlock = true
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		}
		if !inBlock {
			continue
		}
		if v, ok := strings.CutPrefix(trimmed, "hostname:"); ok {
			current.Hostname = strings.Trim(strings.TrimSpace(v), `"'`)
			continue
		}
		if v, ok := strings.CutPrefix(trimmed, "service:"); ok {
			current.Service = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	flush()
	return rules
}

func extractTunnelID(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(trimmed, "tunnel:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// removeIngressRule removes the ingress block whose hostname matches host.
// Lines outside rule blocks (tunnel:, credentials-file:, protocol:, ingress:)
// are preserved verbatim: only the matching rule block is dropped.
func removeIngressRule(text, host string) (string, bool) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	removed := false

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, "- ") {
			out = append(out, line)
			i++
			continue
		}

		// Collect the block: the dash line plus following indented lines.
		block := []string{line}
		j := i + 1
		for j < len(lines) {
			next := lines[j]
			if strings.TrimSpace(next) == "" || !strings.HasPrefix(next, " ") {
				break
			}
			block = append(block, next)
			j++
		}

		if !removed {
			if rules := parseRuleBlocks(strings.Join(block, "\n")); len(rules) > 0 && rules[0].Hostname == host {
				removed = true
				i = j
				continue
			}
		}

		out = append(out, block...)
		i = j
	}

	return strings.Join(out, "\n"), removed
}

// restartAndVerify restarts cloudflared and waits for it to actually be active.
// A restart that is immediately followed by a crash loop must not count as success.
func (s *Server) restartAndVerify() (bool, string) {
	if !s.SystemdEnabled {
		return true, ""
	}
	res := execx.Run([]string{s.SystemctlBin, "restart", "cloudflared.service"}, 90*time.Second)
	if !res.OK() {
		return false, "restart failed: " + firstLine(res.Stderr, res.Err)
	}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		state := strings.TrimSpace(execx.Run([]string{s.SystemctlBin, "is-active", "cloudflared.service"}, 10*time.Second).Stdout)
		if state == "active" {
			return true, ""
		}
		if state == "failed" {
			return false, "cloudflared failed to start"
		}
		time.Sleep(2 * time.Second)
	}
	return false, "cloudflared did not become active"
}

// restoreIngress puts the previous config back and restarts the tunnel.
func (s *Server) restoreIngress(previous string, mode os.FileMode) {
	_ = atomicWrite(s.CloudflaredConfig, []byte(previous), mode)
	if s.SystemdEnabled {
		_ = execx.Run([]string{s.SystemctlBin, "restart", "cloudflared.service"}, 90*time.Second)
	}
}

// insertIngressRule adds a hostname rule just above the catch-all entry,
// reusing whatever indentation the file already uses. Mixing indentation
// styles produces invalid YAML, which cloudflared then refuses to load.
func insertIngressRule(text, host, target string) (string, bool) {
	lines := strings.Split(text, "\n")
	insertAt := -1
	for i, line := range lines {
		if strings.Contains(strings.TrimSpace(line), "http_status:404") {
			insertAt = i
		}
	}
	if insertAt < 0 {
		return "", false
	}

	line := lines[insertAt]
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	block := []string{
		indent + "- hostname: " + host,
		indent + "  service: " + target,
	}

	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:insertAt]...)
	out = append(out, block...)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n"), true
}

func (s *Server) handleCloudflared(w http.ResponseWriter, r *http.Request) {
	page := cfPage{
		BaseData:   withFlash(s.base(r, "Cloudflared"), r),
		ConfigPath: s.CloudflaredConfig,
	}
	if !s.CloudflaredEnabled {
		s.Renderer.Render(w, http.StatusOK, "cloudflared", page)
		return
	}

	text, err := s.readConfig()
	if err != nil {
		page.Error = "无法读取配置文件：" + err.Error()
		s.Renderer.Render(w, http.StatusOK, "cloudflared", page)
		return
	}
	page.Available = true
	page.RawConfig = text
	page.Tunnel = extractTunnelID(text)
	page.Rules = parseRuleBlocks(text)
	s.Renderer.Render(w, http.StatusOK, "cloudflared", page)
}

func (s *Server) validateServiceTarget(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("service 不是合法 URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("service 只允许 http/https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("service 缺少主机名")
	}
	return nil
}

func (s *Server) hostnameAllowed(host string) error {
	if !hostnamePattern.MatchString(host) {
		return fmt.Errorf("hostname 格式不合法")
	}
	if len(s.CloudflaredSuffixAllowlist) == 0 {
		return nil
	}
	for _, suffix := range s.CloudflaredSuffixAllowlist {
		if strings.HasSuffix(host, suffix) {
			return nil
		}
	}
	return fmt.Errorf("hostname 不在允许的域名后缀内")
}

func backupPath(path string) string {
	return path + ".bak." + strconv.FormatInt(time.Now().Unix(), 10)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".zypanel-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *Server) handleCloudflaredAdd(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.TrimSpace(r.FormValue("hostname")))
	target := strings.TrimSpace(r.FormValue("service"))

	if !s.CloudflaredEnabled || s.ReadOnly {
		http.Redirect(w, r, "/cloudflared?err=disabled", http.StatusSeeOther)
		return
	}
	if err := s.hostnameAllowed(host); err != nil {
		s.record(r, "cloudflared.add", host, false, err.Error())
		http.Redirect(w, r, "/cloudflared?err=invalid-hostname", http.StatusSeeOther)
		return
	}
	if err := s.validateServiceTarget(target); err != nil {
		s.record(r, "cloudflared.add", host, false, err.Error())
		http.Redirect(w, r, "/cloudflared?err=invalid-service", http.StatusSeeOther)
		return
	}

	text, err := s.readConfig()
	if err != nil {
		http.Redirect(w, r, "/cloudflared?err=config-unreadable", http.StatusSeeOther)
		return
	}
	for _, rule := range parseRuleBlocks(text) {
		if rule.Hostname == host {
			http.Redirect(w, r, "/cloudflared?err=hostname-exists", http.StatusSeeOther)
			return
		}
	}

	newText, ok := insertIngressRule(text, host, target)
	if !ok {
		http.Redirect(w, r, "/cloudflared?err=catchall-not-found", http.StatusSeeOther)
		return
	}

	info, err := os.Stat(s.CloudflaredConfig)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(backupPath(s.CloudflaredConfig), []byte(text), mode); err != nil {
		s.record(r, "cloudflared.add", host, false, "backup failed")
		http.Redirect(w, r, "/cloudflared?err=backup-failed", http.StatusSeeOther)
		return
	}
	if err := atomicWrite(s.CloudflaredConfig, []byte(newText), mode); err != nil {
		s.record(r, "cloudflared.add", host, false, err.Error())
		http.Redirect(w, r, "/cloudflared?err=write-failed", http.StatusSeeOther)
		return
	}

	detail := "service=" + target
	if execx.LookPath(s.CloudflaredBin) {
		if res := execx.Run([]string{s.CloudflaredBin, "--config", s.CloudflaredConfig, "tunnel", "ingress", "validate"}, 30*time.Second); !res.OK() {
			_ = atomicWrite(s.CloudflaredConfig, []byte(text), mode)
			s.record(r, "cloudflared.add", host, false, "ingress validate failed: "+firstLine(res.Stderr, res.Err))
			http.Redirect(w, r, "/cloudflared?err=validate-failed", http.StatusSeeOther)
			return
		}
	}

	ok, restartDetail := s.restartAndVerify()
	if !ok {
		detail += "; " + restartDetail
		s.restoreIngress(text, mode)
	}
	s.record(r, "cloudflared.add", host, ok, detail)	if ok {
		http.Redirect(w, r, "/cloudflared?ok=rule-added", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/cloudflared?err=restart-failed", http.StatusSeeOther)
}

func (s *Server) handleCloudflaredRemove(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.TrimSpace(r.FormValue("hostname")))
	if !s.CloudflaredEnabled || s.ReadOnly || host == "" {
		http.Redirect(w, r, "/cloudflared?err=disabled", http.StatusSeeOther)
		return
	}

	text, err := s.readConfig()
	if err != nil {
		http.Redirect(w, r, "/cloudflared?err=config-unreadable", http.StatusSeeOther)
		return
	}

	out, removed := removeIngressRule(text, host)
	if !removed {
		http.Redirect(w, r, "/cloudflared?err=hostname-not-found", http.StatusSeeOther)
		return
	}

	info, statErr := os.Stat(s.CloudflaredConfig)
	mode := os.FileMode(0o644)
	if statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(backupPath(s.CloudflaredConfig), []byte(text), mode); err != nil {
		s.record(r, "cloudflared.remove", host, false, "backup failed")
		http.Redirect(w, r, "/cloudflared?err=backup-failed", http.StatusSeeOther)
		return
	}
	if err := atomicWrite(s.CloudflaredConfig, []byte(out), mode); err != nil {
		s.record(r, "cloudflared.remove", host, false, err.Error())
		http.Redirect(w, r, "/cloudflared?err=write-failed", http.StatusSeeOther)
		return
	}

	ok, detail := s.restartAndVerify()
	if ok {
		detail = "removed"
	} else {
		s.restoreIngress(text, mode)
	}
	s.record(r, "cloudflared.remove", host, ok, detail)
	if ok {
		http.Redirect(w, r, "/cloudflared?ok=rule-removed", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/cloudflared?err=restart-failed", http.StatusSeeOther)
}

func (s *Server) handleCloudflaredRestart(w http.ResponseWriter, r *http.Request) {
	if !s.CloudflaredEnabled || s.ReadOnly || !s.SystemdEnabled {
		http.Redirect(w, r, "/cloudflared?err=disabled", http.StatusSeeOther)
		return
	}
	res := execx.Run([]string{s.SystemctlBin, "restart", "cloudflared.service"}, 90*time.Second)
	s.record(r, "cloudflared.restart", "cloudflared.service", res.OK(), firstLine(res.Stderr, res.Err))
	if res.OK() {
		http.Redirect(w, r, "/cloudflared?ok=restarted", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/cloudflared?err=restart-failed", http.StatusSeeOther)
}
