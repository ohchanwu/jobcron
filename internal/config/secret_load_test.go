package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSecretFiles(t *testing.T) {
	env := map[string]string{}
	values := map[string]string{"DATABASE_URL": "synthetic-database", "SESSION_SECRET": strings.Repeat("s", 32), "JOBCRON_CREDENTIAL_ENCRYPTION_KEY": "MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=", "JOBCRON_SIGNUP_ACCESS_CODE": "synthetic-signup", "JOBCRON_PROXY_SECRET": "synthetic-proxy", "JOBCRON_ADMIN_TOKEN": "synthetic-admin", "JOBCRON_WORKNET_KEY": "synthetic-worknet"}
	for name, value := range values {
		path := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		env[name+"_FILE"] = path
	}
	cfg, err := Load(nil, env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != values["DATABASE_URL"] || string(cfg.SessionSecret) != values["SESSION_SECRET"] || cfg.SignupAccessCode != values["JOBCRON_SIGNUP_ACCESS_CODE"] || cfg.ProxySecret != values["JOBCRON_PROXY_SECRET"] || len(cfg.CredentialEncryptionKey) != 32 || cfg.AdminToken != values["JOBCRON_ADMIN_TOKEN"] || cfg.WorknetKey != values["JOBCRON_WORKNET_KEY"] {
		t.Fatal("file inputs not loaded")
	}
	for name := range values {
		env[name] = ""
		if _, err := Load(nil, env); err == nil {
			t.Errorf("accepted ambiguous %s", name)
		}
		delete(env, name)
	}
}
