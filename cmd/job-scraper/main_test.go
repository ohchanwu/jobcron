package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/appdata"
	"github.com/ohchanwu/job-scraper/internal/config"
)

func TestListenFallsBackWhenPortBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy a port: %v", err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	ln, addr, err := listen("127.0.0.1", busyPort)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	if addr == busy.Addr().String() {
		t.Errorf("listen returned the busy address %s; should have fallen back", addr)
	}
}

func TestListenUsesConfiguredHost(t *testing.T) {
	ln, addr, err := listen("0.0.0.0", 0)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "0.0.0.0" {
		t.Fatalf("host = %q, want 0.0.0.0", host)
	}
}

func TestOpenConfiguredStoreProductionUsesPostgresPath(t *testing.T) {
	_, err := openConfiguredStore(config.Config{
		Production:  true,
		DatabaseURL: "://not-a-valid-postgres-url",
	})
	if err == nil {
		t.Fatal("openConfiguredStore succeeded with an invalid PostgreSQL URL")
	}
	if got, want := err.Error(), "storage: open postgres"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("openConfiguredStore error = %v, want PostgreSQL open path error", err)
	}
}

func TestOpenConfiguredStoreDatabaseURLUsesPostgresOutsideProduction(t *testing.T) {
	_, err := openConfiguredStore(config.Config{
		DatabaseURL: "://not-a-valid-postgres-url",
	})
	if err == nil {
		t.Fatal("openConfiguredStore succeeded with an invalid PostgreSQL URL")
	}
	if got, want := err.Error(), "storage: open postgres"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("openConfiguredStore error = %v, want PostgreSQL open path error", err)
	}
}

func TestPrepareApplicationDataMigratesLegacyDirectory(t *testing.T) {
	root := t.TempDir()
	legacyFile := filepath.Join(appdata.LegacyDir(root), "jobs.db")
	if err := os.MkdirAll(filepath.Dir(legacyFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyFile, []byte("database"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := prepareApplicationData(root); err != nil {
		t.Fatalf("prepareApplicationData: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(appdata.Dir(root), "jobs.db"))
	if err != nil || string(got) != "database" {
		t.Fatalf("migrated database = %q, err = %v", got, err)
	}
}
