package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Metrics struct {
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

type Node struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	IP               string    `json:"ip"`
	GroupIDs         []string  `json:"groupIds"`
	GroupID          string    `json:"groupId,omitempty"` // legacy API compatibility
	Sort             int       `json:"sort"`
	AgentToken       string    `json:"agentToken,omitempty"`
	Version          string    `json:"version"`
	LastSeen         time.Time `json:"lastSeen"`
	UpgradeRequested bool      `json:"upgradeRequested,omitempty"`
	TrafficDate      string    `json:"trafficDate"`
	TodayUpload      uint64    `json:"todayUpload"`
	TodayDownload    uint64    `json:"todayDownload"`
	Metrics
}

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

type data struct {
	EnrollmentToken string        `json:"enrollmentToken"`
	AdminToken      string        `json:"adminToken"`
	Groups          []Group       `json:"groups"`
	Nodes           []Node        `json:"nodes"`
	RemovedNodes    []RemovedNode `json:"removedNodes,omitempty"`
}

type RemovedNode struct {
	ID         string `json:"id"`
	AgentToken string `json:"agentToken,omitempty"`
}

var ErrRemovalPending = errors.New("agent removal pending")

// EnsureAdminToken stores the initial token and returns the persisted token.
// A token changed in the dashboard therefore survives service restarts.
func (s *Store) EnsureAdminToken(initial string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.AdminToken == "" {
		s.data.AdminToken = initial
	}
	return s.data.AdminToken, s.saveLocked()
}

func (s *Store) SetAdminToken(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.AdminToken = value
	return s.saveLocked()
}

// EnsureEnrollmentToken returns the shared token used by all new agents. The
// token is stored with panel data so it remains stable across restarts.
func (s *Store) EnsureEnrollmentToken(configured string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if configured != "" {
		s.data.EnrollmentToken = configured
	}
	if s.data.EnrollmentToken == "" {
		s.data.EnrollmentToken = token(24)
	}
	return s.data.EnrollmentToken, s.saveLocked()
}

type Store struct {
	mu   sync.RWMutex
	path string
	data data
	now  func() time.Time
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, data: data{Groups: []Group{}, Nodes: []Node{}}, now: time.Now}
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for i := range s.data.Nodes {
		if len(s.data.Nodes[i].GroupIDs) == 0 && s.data.Nodes[i].GroupID != "" {
			s.data.Nodes[i].GroupIDs = []string{s.data.Nodes[i].GroupID}
		}
		if s.data.Nodes[i].GroupIDs == nil {
			s.data.Nodes[i].GroupIDs = []string{}
		}
		s.data.Nodes[i].GroupID = ""
	}
	return s, nil
}

func token(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Snapshot() ([]Group, []Node) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	groups := append([]Group{}, s.data.Groups...)
	nodes := append([]Node{}, s.data.Nodes...)
	today := s.now().In(trafficLocation).Format("2006-01-02")
	for i := range nodes {
		if nodes[i].TrafficDate != today {
			nodes[i].TrafficDate = today
			nodes[i].TodayUpload, nodes[i].TodayDownload = 0, 0
		}
		nodes[i].AgentToken = ""
		nodes[i].UpgradeRequested = false
		if len(nodes[i].GroupIDs) > 0 {
			nodes[i].GroupID = nodes[i].GroupIDs[0]
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Sort > groups[j].Sort })
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Sort == nodes[j].Sort {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Sort > nodes[j].Sort
	})
	return groups, nodes
}

func (s *Store) CreateNode(name, groupID string, order int) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	groupIDs := []string{}
	if groupID != "" {
		groupIDs = append(groupIDs, groupID)
	}
	n := Node{ID: token(8), Name: name, GroupIDs: groupIDs, Sort: order, AgentToken: token(24)}
	s.data.Nodes = append(s.data.Nodes, n)
	return n, s.saveLocked()
}

func (s *Store) UpdateNode(id, name string, groupIDs []string, order int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Nodes {
		if s.data.Nodes[i].ID == id {
			s.data.Nodes[i].Name, s.data.Nodes[i].GroupIDs, s.data.Nodes[i].Sort = name, unique(groupIDs), order
			s.data.Nodes[i].GroupID = ""
			return s.saveLocked()
		}
	}
	return os.ErrNotExist
}

func (s *Store) DeleteNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Nodes {
		if s.data.Nodes[i].ID == id {
			oldNodes := append([]Node{}, s.data.Nodes...)
			oldRemoved := s.data.RemovedNodes
			s.data.RemovedNodes = append(s.data.RemovedNodes, RemovedNode{ID: id, AgentToken: s.data.Nodes[i].AgentToken})
			s.data.Nodes = append(s.data.Nodes[:i], s.data.Nodes[i+1:]...)
			if err := s.saveLocked(); err != nil {
				s.data.Nodes, s.data.RemovedNodes = oldNodes, oldRemoved
				return err
			}
			return nil
		}
	}
	return os.ErrNotExist
}

func (s *Store) CreateGroup(name string, order int) (Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := Group{ID: token(6), Name: name, Sort: order}
	s.data.Groups = append(s.data.Groups, g)
	return g, s.saveLocked()
}

func (s *Store) UpdateGroup(id, name string, order int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Groups {
		if s.data.Groups[i].ID == id {
			s.data.Groups[i].Name, s.data.Groups[i].Sort = name, order
			return s.saveLocked()
		}
	}
	return os.ErrNotExist
}

// UpdateGroupWithNodes updates membership for this group without changing a
// node's membership in any other group.
func (s *Store) UpdateGroupWithNodes(id, name string, order int, nodeIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for i := range s.data.Groups {
		if s.data.Groups[i].ID == id {
			s.data.Groups[i].Name, s.data.Groups[i].Sort = name, order
			found = true
			break
		}
	}
	if !found {
		return os.ErrNotExist
	}
	selected := make(map[string]bool, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		selected[nodeID] = true
	}
	for i := range s.data.Nodes {
		if selected[s.data.Nodes[i].ID] {
			if !contains(s.data.Nodes[i].GroupIDs, id) {
				s.data.Nodes[i].GroupIDs = append(s.data.Nodes[i].GroupIDs, id)
			}
		} else {
			s.data.Nodes[i].GroupIDs = without(s.data.Nodes[i].GroupIDs, id)
		}
	}
	return s.saveLocked()
}

func (s *Store) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Groups {
		if s.data.Groups[i].ID == id {
			s.data.Groups = append(s.data.Groups[:i], s.data.Groups[i+1:]...)
			for j := range s.data.Nodes {
				s.data.Nodes[j].GroupIDs = without(s.data.Nodes[j].GroupIDs, id)
			}
			return s.saveLocked()
		}
	}
	return os.ErrNotExist
}

func (s *Store) Report(agentToken, ip, version string, metrics Metrics) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, removed := range s.data.RemovedNodes {
		if agentToken != "" && removed.AgentToken == agentToken {
			return false, ErrRemovalPending
		}
	}
	for i := range s.data.Nodes {
		n := &s.data.Nodes[i]
		if agentToken != "" && n.AgentToken == agentToken {
			n.applyReport(ip, version, metrics, s.now())
			upgrade := n.UpgradeRequested
			n.UpgradeRequested = false
			return upgrade, s.saveLocked()
		}
	}
	return false, os.ErrNotExist
}

// AutoReport updates an automatically enrolled node, creating it on the first
// heartbeat. Its display name defaults to the observed IP and dashboard edits win.
func (s *Store) AutoReport(nodeID, name, ip, version string, metrics Metrics) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, removed := range s.data.RemovedNodes {
		if removed.ID == nodeID {
			return false, ErrRemovalPending
		}
	}
	for i := range s.data.Nodes {
		n := &s.data.Nodes[i]
		if n.ID == nodeID {
			if n.Name == "" {
				n.Name = ip
			}
			n.applyReport(ip, version, metrics, s.now())
			upgrade := n.UpgradeRequested
			n.UpgradeRequested = false
			return upgrade, s.saveLocked()
		}
	}
	n := Node{ID: nodeID, Name: ip, GroupIDs: []string{}, Sort: 0}
	n.applyReport(ip, version, metrics, s.now())
	s.data.Nodes = append(s.data.Nodes, n)
	return false, s.saveLocked()
}

var trafficLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Daily traffic is calculated from counter deltas, never from the initial
// lifetime counter. Persisted metrics are also the baseline after a restart.
func (n *Node) applyReport(ip, version string, metrics Metrics, now time.Time) {
	local := now.In(trafficLocation)
	date := local.Format("2006-01-02")
	initialized := n.TrafficDate != "" && !n.LastSeen.IsZero()
	if n.TrafficDate != date {
		n.TodayUpload, n.TodayDownload = 0, 0
	}
	if initialized && now.After(n.LastSeen) {
		start := n.LastSeen
		// Uptime also detects a reboot when new counters already exceed the old ones.
		rebooted := metrics.Uptime < n.Uptime || (n.Uptime > 0 && metrics.Uptime > 0 &&
			float64(metrics.Uptime)+5 < float64(n.Uptime)+now.Sub(n.LastSeen).Seconds())
		if rebooted {
			boot := now.Add(-time.Duration(metrics.Uptime) * time.Second)
			if boot.After(start) {
				start = boot
			}
		}
		delta := func(current, previous uint64) uint64 {
			if rebooted || current < previous {
				return current
			}
			return current - previous
		}
		up, down := delta(metrics.TotalUpload, n.TotalUpload), delta(metrics.TotalDownload, n.TotalDownload)
		midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, trafficLocation)
		if start.Before(midnight) {
			// Reports cannot identify the exact time of each byte. Split intervals
			// that cross midnight proportionally, including gaps while offline.
			ratio := float64(now.Sub(midnight)) / float64(now.Sub(start))
			up, down = uint64(float64(up)*ratio), uint64(float64(down)*ratio)
		}
		n.TodayUpload += up
		n.TodayDownload += down
	}
	n.TrafficDate = date
	n.IP, n.Version, n.LastSeen, n.Metrics = ip, version, now.UTC(), metrics
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func without(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func unique(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (s *Store) RequestUpgrade(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Nodes {
		if s.data.Nodes[i].ID == id {
			s.data.Nodes[i].UpgradeRequested = true
			return s.saveLocked()
		}
	}
	return os.ErrNotExist
}
