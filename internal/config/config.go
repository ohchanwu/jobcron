package config

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	defaultHost            = "127.0.0.1"
	defaultPort            = 7777
	defaultDailyScrapeTime = "08:00"
	minSessionSecretBytes  = 32
)

// Config contains runtime configuration parsed from command-line flags and
// environment variables.
type Config struct {
	Production       bool
	DatabaseURL      string
	SessionSecret    []byte
	SchedulerEnabled bool
	DailyScrapeTime  string
	AdminToken       string
	WorknetKey       string
	Host             string
	Port             int
	NoOpen           bool
	Demo             bool
	DBPath           string
}

// Load parses job-scraper configuration. Existing CLI flags override matching
// environment defaults.
func Load(args []string, env map[string]string) (Config, error) {
	cfg := Config{
		Production:       envValue(env, "JOBSCRAPER_ENV") == "production",
		DatabaseURL:      envValue(env, "DATABASE_URL"),
		SessionSecret:    []byte(envValue(env, "SESSION_SECRET")),
		SchedulerEnabled: envBool(envValue(env, "JOBSCRAPER_SCHEDULER_ENABLED")),
		DailyScrapeTime:  envDefault(env, "JOBSCRAPER_DAILY_SCRAPE_TIME", defaultDailyScrapeTime),
		AdminToken:       envValue(env, "JOBSCRAPER_ADMIN_TOKEN"),
		WorknetKey:       envValue(env, "JOBSCRAPER_WORKNET_KEY"),
		Host:             envDefault(env, "JOBSCRAPER_HOST", defaultHost),
		Port:             envIntDefault(env, "JOBSCRAPER_PORT", defaultPort),
		NoOpen:           envBool(envValue(env, "JOBSCRAPER_NO_OPEN")),
		Demo:             envBool(envValue(env, "JOBSCRAPER_DEMO")),
		DBPath:           envValue(env, "JOBSCRAPER_DB"),
	}

	fs := flag.NewFlagSet("job-scraper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&cfg.Port, "port", cfg.Port, "preferred port; the next ten are tried if it is busy")
	fs.StringVar(&cfg.Host, "host", cfg.Host, "host/interface to bind")
	fs.BoolVar(&cfg.NoOpen, "no-open", cfg.NoOpen, "do not open a browser window on startup")
	fs.BoolVar(&cfg.Demo, "demo", cfg.Demo, "run in read-only public demo mode")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "database file path (default: under the OS config dir)")
	fs.StringVar(&cfg.WorknetKey, "worknet-api-key", cfg.WorknetKey,
		"워크넷 OpenAPI key (free at data.go.kr). Disables the 워크넷 source when empty.")
	fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if cfg.Production {
		if cfg.DatabaseURL == "" {
			return Config{}, fmt.Errorf("production requires DATABASE_URL")
		}
		if len(cfg.SessionSecret) < minSessionSecretBytes {
			return Config{}, fmt.Errorf("production requires SESSION_SECRET with at least %d bytes", minSessionSecretBytes)
		}
	}
	return cfg, nil
}

func envValue(env map[string]string, name string) string {
	if env == nil {
		return ""
	}
	return env[name]
}

func envDefault(env map[string]string, name, fallback string) string {
	if v := envValue(env, name); v != "" {
		return v
	}
	return fallback
}

func envIntDefault(env map[string]string, name string, fallback int) int {
	v := envValue(env, name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
