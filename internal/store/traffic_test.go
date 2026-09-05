package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDailyTraffic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 5, 23, 59, 50, 0, trafficLocation)
	s.now = func() time.Time { return now }
	report := func(up, down, uptime uint64) {
		t.Helper()
		_, err := s.AutoReport("daily-node", "", "192.0.2.1", "v1", Metrics{TotalUpload: up, TotalDownload: down, Uptime: uptime})
		if err != nil {
			t.Fatal(err)
		}
	}
	check := func(up, down uint64, date string) {
		t.Helper()
		_, nodes := s.Snapshot()
		n := nodes[0]
		if n.TodayUpload != up || n.TodayDownload != down || n.TrafficDate != date {
			t.Fatalf("got %d/%d on %s, want %d/%d on %s", n.TodayUpload, n.TodayDownload, n.TrafficDate, up, down, date)
		}
	}
	// Existing lifetime counters must not be attributed to today at installation.
	report(10000, 20000, 1000)
	check(0, 0, "2026-09-05")
	now = now.Add(5 * time.Second)
	report(10100, 20400, 1005)
	check(100, 400, "2026-09-05")
	// Panel restart preserves both totals and the counter baseline.
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	check(100, 400, "2026-09-05")
	now = now.Add(10 * time.Second)
	// Offline nodes show zero after midnight even without a new report.
	check(0, 0, "2026-09-06")
	report(10300, 21000, 1015)
	check(100, 300, "2026-09-06")
	// A reboot does not underflow counters or erase traffic already recorded.
	now = now.Add(3 * time.Second)
	report(20, 40, 2)
	check(120, 340, "2026-09-06")
	now = now.Add(3 * time.Second)
	report(30, 60, 5)
	check(130, 360, "2026-09-06")
	// Counter reset without a reboot (for example, an interface reset).
	now = now.Add(3 * time.Second)
	report(5, 10, 8)
	check(135, 370, "2026-09-06")
}

func TestDailyTrafficLegacyTokenAndMigration(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, trafficLocation)
	s.now = func() time.Time { return now }
	n, err := s.CreateNode("legacy", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate v0.5 data: prior metrics exist, but no daily baseline.
	s.data.Nodes[0].LastSeen = now.Add(-time.Hour)
	s.data.Nodes[0].Metrics = Metrics{TotalUpload: 5000, TotalDownload: 10000}
	if _, err := s.Report(n.AgentToken, "192.0.2.2", "v1", Metrics{TotalUpload: 9000, TotalDownload: 18000}); err != nil {
		t.Fatal(err)
	}
	_, nodes := s.Snapshot()
	if nodes[0].TodayUpload != 0 || nodes[0].TodayDownload != 0 {
		t.Fatal("migration counted historical traffic")
	}
	now = now.Add(3 * time.Second)
	if _, err := s.Report(n.AgentToken, "192.0.2.2", "v1", Metrics{TotalUpload: 9500, TotalDownload: 19000}); err != nil {
		t.Fatal(err)
	}
	_, nodes = s.Snapshot()
	if nodes[0].TodayUpload != 500 || nodes[0].TodayDownload != 1000 {
		t.Fatal("legacy reports did not accumulate traffic")
	}
}
