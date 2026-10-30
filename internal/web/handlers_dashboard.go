package web

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/EisonMe/zypanel/internal/execx"
)

type hostStats struct {
	Hostname  string
	Uptime    string
	Load      string
	MemUsed   string
	MemTotal  string
	MemPct    int
	DiskUsed  string
	DiskTotal string
	DiskPct   int
}

type dashboardPage struct {
	BaseData
	Stats      hostStats
	DiskOK     bool
	Containers int
	Running    int
	Services   int
	Failed     int
	DockerErr  string
	SystemdErr string
}

func readUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "-"
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "-"
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "-"
	}
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h"
	}
	return strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
}

func readLoad() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "-"
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return "-"
	}
	return fields[0] + " / " + fields[1] + " / " + fields[2]
}

func readMem() (usedMB, totalMB int, pct int) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	vals := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		if v, err := strconv.Atoi(parts[1]); err == nil {
			vals[key] = v
		}
	}
	total := vals["MemTotal"]
	available := vals["MemAvailable"]
	if total == 0 {
		return 0, 0, 0
	}
	used := total - available
	pct = used * 100 / total
	return used / 1024, total / 1024, pct
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	usedMB, totalMB, memPct := readMem()
	diskUsed, diskTotal, diskPct, diskOK := readDisk("/")

	page := dashboardPage{
		BaseData: withFlash(s.base(r, "Dashboard"), r),
		Stats: hostStats{
			Hostname:  hostname,
			Uptime:    readUptime(),
			Load:      readLoad(),
			MemUsed:   strconv.Itoa(usedMB) + " MB",
			MemTotal:  strconv.Itoa(totalMB) + " MB",
			MemPct:    memPct,
			DiskUsed:  strconv.Itoa(diskUsed) + " GB",
			DiskTotal: strconv.Itoa(diskTotal) + " GB",
			DiskPct:   diskPct,
		},
		DiskOK: diskOK,
	}

	if s.DockerEnabled {
		res := execx.Run([]string{s.DockerBin, "ps", "-a", "--format", "{{.State}}"}, 10*time.Second)
		if res.OK() {
			for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				page.Containers++
				if line == "running" {
					page.Running++
				}
			}
		} else {
			page.DockerErr = firstLine(res.Stderr, res.Err)
		}
	}

	if s.SystemdEnabled {
		res := execx.Run([]string{s.SystemctlBin, "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager"}, 10*time.Second)
		if res.OK() {
			for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
				if strings.TrimSpace(line) != "" {
					page.Services++
				}
			}
		} else {
			page.SystemdErr = firstLine(res.Stderr, res.Err)
		}
		res = execx.Run([]string{s.SystemctlBin, "list-units", "--type=service", "--state=failed", "--no-legend", "--no-pager"}, 10*time.Second)
		if res.OK() {
			for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
				if strings.TrimSpace(line) != "" {
					page.Failed++
				}
			}
		}
	}

	s.Renderer.Render(w, http.StatusOK, "dashboard", page)
}

func firstLine(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			if i := strings.IndexByte(v, '\n'); i > 0 {
				return v[:i]
			}
			return v
		}
	}
	return ""
}
