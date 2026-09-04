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
	GroupID          string    `json:"groupId"`
	Sort             int       `json:"sort"`
	AgentToken       string    `json:"agentToken,omitempty"`
	Version          string    `json:"version"`
	LastSeen         time.Time `json:"lastSeen"`
	UpgradeRequested bool      `json:"upgradeRequested,omitempty"`
	Metrics
}

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

type data struct {
	Groups []Group `json:"groups"`
	Nodes  []Node  `json:"nodes"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	data data
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, data: data{Groups: []Group{}, Nodes: []Node{}}}
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
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
	for i := range nodes {
		nodes[i].AgentToken = ""
		nodes[i].UpgradeRequested = false
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Sort < groups[j].Sort })
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Sort == nodes[j].Sort {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Sort < nodes[j].Sort
	})
	return groups, nodes
}

func (s *Store) CreateNode(name, groupID string, order int) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := Node{ID: token(8), Name: name, GroupID: groupID, Sort: order, AgentToken: token(24)}
	s.data.Nodes = append(s.data.Nodes, n)
	return n, s.saveLocked()
}

func (s *Store) UpdateNode(id, name, groupID string, order int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Nodes {
		if s.data.Nodes[i].ID == id {
			s.data.Nodes[i].Name, s.data.Nodes[i].GroupID, s.data.Nodes[i].Sort = name, groupID, order
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
			s.data.Nodes = append(s.data.Nodes[:i], s.data.Nodes[i+1:]...)
			return s.saveLocked()
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

func (s *Store) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Groups {
		if s.data.Groups[i].ID == id {
			s.data.Groups = append(s.data.Groups[:i], s.data.Groups[i+1:]...)
			for j := range s.data.Nodes {
				if s.data.Nodes[j].GroupID == id {
					s.data.Nodes[j].GroupID = ""
				}
			}
			return s.saveLocked()
		}
	}
	return os.ErrNotExist
}

func (s *Store) Report(agentToken, ip, version string, metrics Metrics) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Nodes {
		n := &s.data.Nodes[i]
		if n.AgentToken == agentToken {
			n.IP, n.Version, n.LastSeen, n.Metrics = ip, version, time.Now().UTC(), metrics
			upgrade := n.UpgradeRequested
			n.UpgradeRequested = false
			return upgrade, s.saveLocked()
		}
	}
	return false, os.ErrNotExist
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
