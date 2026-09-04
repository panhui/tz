package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/panhui/tz/internal/assets"
	"github.com/panhui/tz/internal/store"
)

var version = "dev"

type server struct {
	store      *store.Store
	adminToken string
	agentToken string
}

func main() {
	addr := env("TZ_LISTEN", ":8080")
	dataFile := env("TZ_DATA", "/var/lib/tz/data.json")
	adminToken := os.Getenv("TZ_ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = randomAdminToken()
		log.Printf("TZ_ADMIN_TOKEN 未设置，本次启动的管理令牌：%s", adminToken)
	}
	s, err := store.Open(dataFile)
	if err != nil {
		log.Fatal(err)
	}
	agentToken, err := s.EnsureEnrollmentToken(os.Getenv("TZ_AGENT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}
	app := &server{store: s, adminToken: adminToken, agentToken: agentToken}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", app.api)
	mux.HandleFunc("/install.sh", installScript)
	web, _ := fs.Sub(assets.Files, "web")
	mux.Handle("/", http.FileServer(http.FS(web)))
	h := securityHeaders(requestLog(mux))
	log.Printf("TZ Panel %s 正在监听 %s", version, addr)
	log.Fatal(http.ListenAndServe(addr, h))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomAdminToken() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("tz-%d", time.Now().UnixNano())
	}
	return "tz-" + hex.EncodeToString(b)
}

func installScript(w http.ResponseWriter, r *http.Request) {
	b, err := assets.Files.ReadFile("scripts/install-agent.sh")
	if err != nil {
		http.Error(w, "installer unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Write(b)
}

func (s *server) api(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	if path == "report" {
		s.report(w, r)
		return
	}
	if !s.authorized(r) {
		jsonError(w, "管理令牌无效", http.StatusUnauthorized)
		return
	}
	parts := strings.Split(path, "/")
	switch {
	case path == "dashboard" && r.Method == http.MethodGet:
		groups, nodes := s.store.Snapshot()
		writeJSON(w, map[string]any{"groups": groups, "nodes": nodes, "serverTime": time.Now().UTC()})
	case path == "install" && r.Method == http.MethodGet:
		writeJSON(w, map[string]string{"agentToken": s.agentToken})
	case path == "nodes" && r.Method == http.MethodPost:
		var in struct {
			Name, GroupID string
			Sort          int
		}
		if !decode(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			jsonError(w, "服务器名称不能为空", http.StatusBadRequest)
			return
		}
		n, err := s.store.CreateNode(strings.TrimSpace(in.Name), in.GroupID, in.Sort)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		publicNode := n
		publicNode.AgentToken = ""
		writeJSON(w, map[string]any{"node": publicNode, "agentToken": n.AgentToken})
	case len(parts) == 2 && parts[0] == "nodes" && r.Method == http.MethodPut:
		var in struct {
			Name, GroupID string
			Sort          int
		}
		if !decode(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			jsonError(w, "服务器名称不能为空", http.StatusBadRequest)
			return
		}
		respondErr(w, s.store.UpdateNode(parts[1], strings.TrimSpace(in.Name), in.GroupID, in.Sort))
	case len(parts) == 2 && parts[0] == "nodes" && r.Method == http.MethodDelete:
		respondErr(w, s.store.DeleteNode(parts[1]))
	case len(parts) == 3 && parts[0] == "nodes" && parts[2] == "upgrade" && r.Method == http.MethodPost:
		respondErr(w, s.store.RequestUpgrade(parts[1]))
	case path == "groups" && r.Method == http.MethodPost:
		var in struct {
			Name string
			Sort int
		}
		if !decode(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			jsonError(w, "分组名称不能为空", http.StatusBadRequest)
			return
		}
		g, err := s.store.CreateGroup(strings.TrimSpace(in.Name), in.Sort)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		writeJSON(w, g)
	case len(parts) == 2 && parts[0] == "groups" && r.Method == http.MethodPut:
		var in struct {
			Name string
			Sort int
		}
		if !decode(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			jsonError(w, "分组名称不能为空", http.StatusBadRequest)
			return
		}
		respondErr(w, s.store.UpdateGroup(parts[1], strings.TrimSpace(in.Name), in.Sort))
	case len(parts) == 2 && parts[0] == "groups" && r.Method == http.MethodDelete:
		respondErr(w, s.store.DeleteGroup(parts[1]))
	default:
		jsonError(w, "接口不存在", http.StatusNotFound)
	}
}

func (s *server) report(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	token := r.Header.Get("X-Agent-Token")
	var in struct {
		NodeID  string `json:"nodeId"`
		Name    string `json:"name"`
		Version string `json:"version"`
		store.Metrics
	}
	if !decode(w, r, &in) {
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		host = forwarded
	}
	var upgrade bool
	if token == s.agentToken && validNodeID(in.NodeID) {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "Linux 节点"
		}
		if len(name) > 60 {
			name = name[:60]
		}
		upgrade, err = s.store.AutoReport(in.NodeID, name, host, in.Version, in.Metrics)
	} else {
		// Keep existing per-node tokens working during migration.
		upgrade, err = s.store.Report(token, host, in.Version, in.Metrics)
	}
	if err != nil {
		jsonError(w, "探针令牌无效", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "upgrade": upgrade})
}

func validNodeID(id string) bool {
	if len(id) < 8 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func (s *server) authorized(r *http.Request) bool {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") == s.adminToken
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		jsonError(w, "请求内容无效", 400)
		return false
	}
	return true
}

func respondErr(w http.ResponseWriter, err error) {
	if err != nil {
		jsonError(w, "记录不存在", 404)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	writeJSON(w, map[string]string{"error": message})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'")
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
