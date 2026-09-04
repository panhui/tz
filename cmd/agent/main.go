package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

type metrics struct {
	CPU           float64 `json:"cpu"`
	MemoryUsed    uint64  `json:"memoryUsed"`
	MemoryTotal   uint64  `json:"memoryTotal"`
	DiskUsed      uint64  `json:"diskUsed"`
	DiskTotal     uint64  `json:"diskTotal"`
	UploadSpeed   uint64  `json:"uploadSpeed"`
	DownloadSpeed uint64  `json:"downloadSpeed"`
	TotalUpload   uint64  `json:"totalUpload"`
	TotalDownload uint64  `json:"totalDownload"`
	Uptime        uint64  `json:"uptime"`
}

type cpuSample struct{ idle, total uint64 }
type netSample struct {
	tx, rx uint64
	at     time.Time
}

func main() {
	panel := flag.String("url", os.Getenv("TZ_PANEL_URL"), "面板地址")
	token := flag.String("token", os.Getenv("TZ_AGENT_TOKEN"), "探针令牌")
	interval := flag.Duration("interval", 3*time.Second, "上报间隔")
	showVersion := flag.Bool("version", false, "显示版本")
	once := flag.Bool("once", false, "仅上报一次")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *panel == "" || *token == "" {
		log.Fatal("必须设置 --url 和 --token")
	}
	*panel = strings.TrimRight(*panel, "/")
	client := &http.Client{Timeout: 10 * time.Second}
	prevNet, _ := readNetwork()
	for {
		m, currentNet, err := collect(prevNet)
		if err != nil {
			log.Printf("采集失败：%v", err)
		} else {
			upgrade, err := report(client, *panel, *token, m)
			if err != nil {
				log.Printf("上报失败：%v", err)
			} else if upgrade {
				log.Printf("收到升级指令")
				go selfUpgrade(*panel, *token)
			}
			prevNet = currentNet
		}
		if *once {
			return
		}
		time.Sleep(*interval)
	}
}

func collect(prev netSample) (metrics, netSample, error) {
	var m metrics
	c1, err := readCPU()
	if err != nil {
		return m, prev, err
	}
	time.Sleep(220 * time.Millisecond)
	c2, err := readCPU()
	if err != nil {
		return m, prev, err
	}
	dTotal, dIdle := c2.total-c1.total, c2.idle-c1.idle
	if dTotal > 0 {
		m.CPU = float64(dTotal-dIdle) * 100 / float64(dTotal)
	}
	m.MemoryUsed, m.MemoryTotal, _ = readMemory()
	m.DiskUsed, m.DiskTotal, _ = readDisk("/")
	now, _ := readNetwork()
	elapsed := now.at.Sub(prev.at).Seconds()
	if elapsed > 0 && now.tx >= prev.tx && now.rx >= prev.rx {
		m.UploadSpeed = uint64(float64(now.tx-prev.tx) / elapsed)
		m.DownloadSpeed = uint64(float64(now.rx-prev.rx) / elapsed)
	}
	m.TotalUpload, m.TotalDownload = now.tx, now.rx
	m.Uptime = readUptime()
	return m, now, nil
}

func readCPU() (cpuSample, error) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if len(f) < 5 {
		return cpuSample{}, errors.New("/proc/stat 格式无效")
	}
	var vals []uint64
	for _, v := range f[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		vals = append(vals, n)
	}
	var total uint64
	for _, v := range vals {
		total += v
	}
	idle := vals[3]
	if len(vals) > 4 {
		idle += vals[4]
	}
	return cpuSample{idle: idle, total: total}, nil
}

func readMemory() (uint64, uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var total, available uint64
	s := bufio.NewScanner(f)
	for s.Scan() {
		p := strings.Fields(s.Text())
		if len(p) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(p[1], 10, 64)
		if p[0] == "MemTotal:" {
			total = v * 1024
		}
		if p[0] == "MemAvailable:" {
			available = v * 1024
		}
	}
	return total - available, total, s.Err()
}

func readDisk(path string) (uint64, uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	return total - free, total, nil
}

func readNetwork() (netSample, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return netSample{}, err
	}
	defer f.Close()
	var rx, tx uint64
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.Fields(strings.NewReplacer(":", " ").Replace(line))
		if len(parts) < 17 || parts[0] == "lo" {
			continue
		}
		r, _ := strconv.ParseUint(parts[1], 10, 64)
		t, _ := strconv.ParseUint(parts[9], 10, 64)
		rx += r
		tx += t
	}
	return netSample{tx: tx, rx: rx, at: time.Now()}, s.Err()
}

func readUptime() uint64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	v, _ := strconv.ParseFloat(f[0], 64)
	return uint64(v)
}

func report(client *http.Client, panel, token string, m metrics) (bool, error) {
	payload := struct {
		Version string `json:"version"`
		metrics
	}{version, m}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, panel+"/api/report", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		text, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, text)
	}
	var out struct {
		Upgrade bool `json:"upgrade"`
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out.Upgrade, err
}

func selfUpgrade(panel, token string) {
	script := fmt.Sprintf("curl -fsSL %q/install.sh | bash -s -- --url %q --token %q --upgrade", panel, panel, token)
	unit := fmt.Sprintf("tz-agent-upgrade-%d", time.Now().Unix())
	cmd := exec.Command("systemd-run", "--unit="+unit, "--collect", "bash", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("升级失败：%v", err)
	}
}
