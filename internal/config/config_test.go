package config_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ohchanwu/jobcron/internal/config"
	"github.com/ohchanwu/jobcron/internal/credential"
)

func TestLoadProductionRequiresDatabaseURL(t *testing.T) {
	env := validProductionEnv()
	delete(env, "DATABASE_URL")
	_, err := config.Load(nil, env)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load error = %v, want DATABASE_URL requirement", err)
	}
}

func TestLoadProductionRequiresSessionSecret(t *testing.T) {
	env := validProductionEnv()
	delete(env, "SESSION_SECRET")
	_, err := config.Load(nil, env)
	if err == nil || !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Fatalf("Load error = %v, want SESSION_SECRET requirement", err)
	}
}

func TestLoadProductionRequiresCredentialEncryptionKey(t *testing.T) {
	env := validProductionEnv()
	delete(env, "JOBCRON_CREDENTIAL_ENCRYPTION_KEY")

	_, err := config.Load(nil, env)
	if err == nil || !strings.Contains(err.Error(), "JOBCRON_CREDENTIAL_ENCRYPTION_KEY") {
		t.Fatalf("Load error = %v, want credential encryption key requirement", err)
	}
}

func TestLoadRejectsInvalidCredentialEncryptionKey(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{name: "malformed base64", encoded: "not-valid-base64!"},
		{name: "thirty-one bytes", encoded: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 31))},
		{name: "thirty-three bytes", encoded: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 33))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validProductionEnv()
			env["JOBCRON_CREDENTIAL_ENCRYPTION_KEY"] = tt.encoded
			_, err := config.Load(nil, env)
			if err == nil {
				t.Fatal("Load succeeded, want credential encryption key error")
			}
			if !strings.Contains(err.Error(), "JOBCRON_CREDENTIAL_ENCRYPTION_KEY") {
				t.Fatalf("Load error = %v, want variable name", err)
			}
			if strings.Contains(err.Error(), tt.encoded) {
				t.Fatalf("Load error %q contains encoded key", err)
			}
		})
	}
}

func TestLoadParsesCredentialEncryptionKey(t *testing.T) {
	env := validProductionEnv()
	want := bytes.Repeat([]byte{0x42}, credential.MasterKeyBytes)

	cfg, err := config.Load(nil, env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !bytes.Equal(cfg.CredentialEncryptionKey, want) {
		t.Fatalf("CredentialEncryptionKey = %x, want %x", cfg.CredentialEncryptionKey, want)
	}
}

func TestLoadAllowsMissingCredentialEncryptionKeyOutsideProduction(t *testing.T) {
	cfg, err := config.Load(nil, map[string]string{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CredentialEncryptionKey != nil {
		t.Fatalf("CredentialEncryptionKey = %x, want nil", cfg.CredentialEncryptionKey)
	}
}

func TestLoadRejectsExplicitInvalidCredentialEncryptionKeyOutsideProduction(t *testing.T) {
	encoded := "local-invalid-key-material"
	_, err := config.Load(nil, map[string]string{
		"JOBCRON_CREDENTIAL_ENCRYPTION_KEY": encoded,
	})
	if err == nil || !strings.Contains(err.Error(), "JOBCRON_CREDENTIAL_ENCRYPTION_KEY") {
		t.Fatalf("Load error = %v, want credential encryption key error", err)
	}
	if strings.Contains(err.Error(), encoded) {
		t.Fatalf("Load error %q contains encoded key", err)
	}
}

func TestLoadVersionBypassesProductionCredentialRequirements(t *testing.T) {
	cfg, err := config.Load([]string{"--version"}, map[string]string{
		"JOBCRON_ENV":                       "production",
		"JOBCRON_CREDENTIAL_ENCRYPTION_KEY": "invalid-key-that-version-must-not-parse",
	})
	if err != nil {
		t.Fatalf("Load --version: %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatal("ShowVersion = false, want true")
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

func TestLoadParsesVersionFlag(t *testing.T) {
	tests := [][]string{
		{"--version"},
		{"-version"},
		{"--version=true"},
		{"-version=true"},
	}
	for _, args := range tests {
		cfg, err := config.Load(args, map[string]string{})
		if err != nil {
			t.Fatalf("Load(%v): %v", args, err)
		}
		if !cfg.ShowVersion {
			t.Fatalf("Load(%v).ShowVersion = false, want true", args)
		}
	}
}

func TestLoadParsesVersionFalseForms(t *testing.T) {
	tests := [][]string{
		{"--version=false"},
		{"-version=false"},
	}
	for _, args := range tests {
		cfg, err := config.Load(args, map[string]string{})
		if err != nil {
			t.Fatalf("Load(%v): %v", args, err)
		}
		if cfg.ShowVersion {
			t.Fatalf("Load(%v).ShowVersion = true, want false", args)
		}
	}
}

func TestLoadInvalidEnvPortReturnsError(t *testing.T) {
	_, err := config.Load(nil, map[string]string{"JOBCRON_PORT": "abc"})
	if err == nil || !strings.Contains(err.Error(), "JOBCRON_PORT") {
		t.Fatalf("Load error = %v, want JOBCRON_PORT error", err)
	}
}

func TestLoadCLIFlagsOverrideEnvironment(t *testing.T) {
	env := map[string]string{
		"JOBCRON_DEMO":         "1",
		"JOBCRON_PROXY_SECRET": "proxy-secret",
		"JOBCRON_WORKNET_KEY":  "env-key",
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
	if cfg.ProxySecret != "proxy-secret" {
		t.Fatalf("ProxySecret = %q, want proxy-secret", cfg.ProxySecret)
	}
	if cfg.WorknetKey != "flag-key" {
		t.Fatalf("WorknetKey = %q, want flag-key", cfg.WorknetKey)
	}
}

func TestLoadIgnoresLegacyEnvironmentPrefix(t *testing.T) {
	cfg, err := config.Load(nil, map[string]string{
		"JOBSCRAPER_ENV":  "production",
		"JOBSCRAPER_PORT": "9000",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Production {
		t.Fatal("legacy JOBSCRAPER_ENV enabled production")
	}
	if cfg.Port == 9000 {
		t.Fatal("legacy JOBSCRAPER_PORT changed port")
	}
}

func TestLoadInvalidFlagPortReturnsError(t *testing.T) {
	_, err := config.Load([]string{"--port", "abc"}, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "invalid value") {
		t.Fatalf("Load error = %v, want invalid port error", err)
	}
}

func validProductionEnv() map[string]string {
	return map[string]string{
		"JOBCRON_ENV":    "production",
		"DATABASE_URL":   "postgres://db.example.invalid/jobs",
		"SESSION_SECRET": strings.Repeat("s", 32),
		"JOBCRON_CREDENTIAL_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(
			bytes.Repeat([]byte{0x42}, credential.MasterKeyBytes),
		),
	}
}
