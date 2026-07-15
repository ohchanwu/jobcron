package config

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ohchanwu/jobcron/internal/credential"
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
	Production              bool
	DatabaseURL             string
	SessionSecret           []byte
	CredentialEncryptionKey []byte
	SchedulerEnabled        bool
	DailyScrapeTime         string
	AdminToken              string
	ProxySecret             string
	WorknetKey              string
	Host                    string
	Port                    int
	StrictPort              bool
	NoOpen                  bool
	Demo                    bool
	ShowHelp                bool
	ShowVersion             bool
}

// Load parses jobcron configuration. Existing CLI flags override matching
// environment defaults.
func Load(args []string, env map[string]string) (Config, error) {
	encodedCredentialEncryptionKey := envValue(env, "JOBCRON_CREDENTIAL_ENCRYPTION_KEY")
	cfg := Config{
		Production:       envValue(env, "JOBCRON_ENV") == "production",
		DatabaseURL:      envValue(env, "DATABASE_URL"),
		SessionSecret:    []byte(envValue(env, "SESSION_SECRET")),
		SchedulerEnabled: envBool(envValue(env, "JOBCRON_SCHEDULER_ENABLED")),
		DailyScrapeTime:  envDefault(env, "JOBCRON_DAILY_SCRAPE_TIME", defaultDailyScrapeTime),
		AdminToken:       envValue(env, "JOBCRON_ADMIN_TOKEN"),
		ProxySecret:      envValue(env, "JOBCRON_PROXY_SECRET"),
		WorknetKey:       envValue(env, "JOBCRON_WORKNET_KEY"),
		Host:             envDefault(env, "JOBCRON_HOST", defaultHost),
		Port:             defaultPort,
		StrictPort:       envBool(envValue(env, "JOBCRON_STRICT_PORT")),
		NoOpen:           envBool(envValue(env, "JOBCRON_NO_OPEN")),
		Demo:             envBool(envValue(env, "JOBCRON_DEMO")),
	}
	if v := envValue(env, "JOBCRON_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("JOBCRON_PORT: %w", err)
		}
		cfg.Port = port
	}

	fs := newFlagSet(&cfg, io.Discard)
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.ShowHelp || cfg.ShowVersion {
		return cfg, nil
	}
	if encodedCredentialEncryptionKey != "" {
		key, err := credential.ParseMasterKey(encodedCredentialEncryptionKey)
		if err != nil {
			return Config{}, fmt.Errorf("JOBCRON_CREDENTIAL_ENCRYPTION_KEY: %w", err)
		}
		cfg.CredentialEncryptionKey = key
	}

	if cfg.Production {
		if cfg.DatabaseURL == "" {
			return Config{}, fmt.Errorf("production requires DATABASE_URL")
		}
		if len(cfg.SessionSecret) < minSessionSecretBytes {
			return Config{}, fmt.Errorf("production requires SESSION_SECRET with at least %d bytes", minSessionSecretBytes)
		}
		if len(cfg.CredentialEncryptionKey) != credential.MasterKeyBytes {
			return Config{}, fmt.Errorf("production requires JOBCRON_CREDENTIAL_ENCRYPTION_KEY with exactly %d decoded bytes", credential.MasterKeyBytes)
		}
	}
	return cfg, nil
}

func newFlagSet(cfg *Config, output io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("jobcron", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.IntVar(&cfg.Port, "port", cfg.Port, "preferred port; the next ten are tried if it is busy")
	fs.StringVar(&cfg.Host, "host", cfg.Host, "host/interface to bind")
	fs.BoolVar(&cfg.NoOpen, "no-open", cfg.NoOpen, "do not open a browser window on startup")
	fs.BoolVar(&cfg.Demo, "demo", cfg.Demo, "run in read-only public demo mode")
	fs.StringVar(&cfg.WorknetKey, "worknet-api-key", cfg.WorknetKey,
		"워크넷 OpenAPI key (free at data.go.kr). Disables the 워크넷 source when empty.")
	fs.BoolVar(&cfg.ShowHelp, "help", false, "print this help and exit")
	fs.BoolVar(&cfg.ShowHelp, "h", false, "print this help and exit")
	fs.BoolVar(&cfg.ShowVersion, "version", cfg.ShowVersion, "print the version and exit")
	return fs
}

// WriteHelp prints the real registered command flags without resolving any
// runtime dependencies.
func WriteHelp(w io.Writer) {
	cfg := Config{Host: defaultHost, Port: defaultPort, DailyScrapeTime: defaultDailyScrapeTime}
	fs := newFlagSet(&cfg, w)
	fmt.Fprintln(w, "Usage: jobcron [flags]")
	fs.PrintDefaults()
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

func envBool(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
