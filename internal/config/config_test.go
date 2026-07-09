package config_test

import (
	"strings"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/config"
)

func TestLoadProductionRequiresDatabaseURL(t *testing.T) {
	env := map[string]string{"JOBSCRAPER_ENV": "production", "SESSION_SECRET": strings.Repeat("a", 32)}
	_, err := config.Load(nil, env)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load error = %v, want DATABASE_URL requirement", err)
	}
}

func TestLoadProductionRequiresSessionSecret(t *testing.T) {
	env := map[string]string{"JOBSCRAPER_ENV": "production", "DATABASE_URL": "postgres://db.example.invalid/jobs"}
	_, err := config.Load(nil, env)
	if err == nil || !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Fatalf("Load error = %v, want SESSION_SECRET requirement", err)
	}
}

func TestLoadDefaultsSchedulerOffOutsideProduction(t *testing.T) {
	cfg, err := config.Load(nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Production {
		t.Fatal("Production = true, want false")
	}
	if cfg.SchedulerEnabled {
		t.Fatal("SchedulerEnabled = true, want false")
	}
}

func TestLoadUsesExplicitDefaults(t *testing.T) {
	cfg, err := config.Load(nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port != 7777 {
		t.Fatalf("Port = %d, want 7777", cfg.Port)
	}
	if cfg.DailyScrapeTime != "08:00" {
		t.Fatalf("DailyScrapeTime = %q, want 08:00", cfg.DailyScrapeTime)
	}
}

func TestLoadCLIFlagsOverrideEnvironment(t *testing.T) {
	env := map[string]string{
		"JOBSCRAPER_DEMO":        "1",
		"JOBSCRAPER_WORKNET_KEY": "env-key",
	}
	cfg, err := config.Load([]string{
		"--host", "0.0.0.0",
		"--port", "8888",
		"--no-open",
		"--demo=false",
		"--db", "/tmp/jobs.db",
		"--worknet-api-key", "flag-key",
	}, env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("Host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != 8888 {
		t.Fatalf("Port = %d, want 8888", cfg.Port)
	}
	if !cfg.NoOpen {
		t.Fatal("NoOpen = false, want true")
	}
	if cfg.Demo {
		t.Fatal("Demo = true, want false")
	}
	if cfg.DBPath != "/tmp/jobs.db" {
		t.Fatalf("DBPath = %q, want /tmp/jobs.db", cfg.DBPath)
	}
	if cfg.WorknetKey != "flag-key" {
		t.Fatalf("WorknetKey = %q, want flag-key", cfg.WorknetKey)
	}
}
