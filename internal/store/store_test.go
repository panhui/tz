package store

import (
	"path/filepath"
	"testing"
)

func TestNodeLifecycleAndReport(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.CreateGroup("生产环境", 1)
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.CreateNode("web-01", g.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	m := Metrics{CPU: 12.5, MemoryUsed: 100, MemoryTotal: 200, UploadSpeed: 10, DownloadSpeed: 20}
	upgrade, err := s.Report(n.AgentToken, "203.0.113.10", "v1", m)
	if err != nil || upgrade {
		t.Fatalf("unexpected report result: upgrade=%v err=%v", upgrade, err)
	}
	if err := s.RequestUpgrade(n.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	s = reopened
	upgrade, err = s.Report(n.AgentToken, "203.0.113.10", "v1", m)
	if err != nil || !upgrade {
		t.Fatalf("expected upgrade command: upgrade=%v err=%v", upgrade, err)
	}
	_, nodes := s.Snapshot()
	if len(nodes) != 1 || nodes[0].IP != "203.0.113.10" || nodes[0].CPU != 12.5 {
		t.Fatalf("unexpected node: %#v", nodes)
	}
	if err := s.DeleteGroup(g.ID); err != nil {
		t.Fatal(err)
	}
	_, nodes = s.Snapshot()
	if nodes[0].GroupID != "" {
		t.Fatal("deleted group should detach nodes")
	}
}

func TestEmptySnapshotUsesArrays(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	groups, nodes := s.Snapshot()
	if groups == nil || nodes == nil {
		t.Fatal("empty snapshots must be non-nil arrays")
	}
}

func TestSharedEnrollmentAutoCreatesAndPreservesNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	enrollmentToken, err := s.EnsureEnrollmentToken("")
	if err != nil || enrollmentToken == "" {
		t.Fatalf("expected generated enrollment token: %q %v", enrollmentToken, err)
	}
	m := Metrics{CPU: 25, MemoryUsed: 50, MemoryTotal: 100}
	if _, err := s.AutoReport("node-12345678", "host-a", "198.51.100.8", "v2", m); err != nil {
		t.Fatal(err)
	}
	_, nodes := s.Snapshot()
	if len(nodes) != 1 || nodes[0].Name != "host-a" || nodes[0].IP != "198.51.100.8" {
		t.Fatalf("unexpected auto-enrolled node: %#v", nodes)
	}
	if err := s.UpdateNode(nodes[0].ID, "自定义名称", "", 9); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AutoReport("node-12345678", "host-a-renamed", "198.51.100.9", "v2", m); err != nil {
		t.Fatal(err)
	}
	_, nodes = s.Snapshot()
	if nodes[0].Name != "自定义名称" {
		t.Fatalf("agent heartbeat overwrote dashboard name: %q", nodes[0].Name)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	persistedToken, err := reopened.EnsureEnrollmentToken("")
	if err != nil || persistedToken != enrollmentToken {
		t.Fatalf("enrollment token did not persist: %q != %q", persistedToken, enrollmentToken)
	}
}
