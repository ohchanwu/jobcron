package production

import (
	"os"
	"strings"
	"testing"
)

func TestProductionFileSecretContract(t *testing.T) {
	config := renderCompose(t)
	app := config.Services["app"]
	for _, name := range []string{"DATABASE_URL", "SESSION_SECRET", credentialKeyEnvName, "JOBCRON_SIGNUP_ACCESS_CODE", proxySecretEnvName} {
		if _, ok := app.Environment[name]; ok {
			t.Errorf("secret value key persisted: %s", name)
		}
		if app.Environment[name+"_FILE"] != "/run/jobcron/secrets/"+name {
			t.Errorf("missing fixed file reference: %s", name)
		}
	}
	for name := range config.Services["caddy"].Environment {
		if strings.Contains(name, "SECRET") {
			t.Error("Caddy must import file, not environment secret")
		}
	}
	for _, v := range config.Services["caddy"].Volumes {
		if v.Type == "volume" {
			t.Error("Caddy persistent volume forbidden")
		}
	}
	found := false
	for _, v := range app.Volumes {
		if v.Source == "/run/jobcron/secrets" && v.Target == v.Source && v.ReadOnly {
			found = true
		}
	}
	if !found {
		t.Error("missing read-only secret directory")
	}
	b, err := os.ReadFile("Caddyfile")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"persist_config off", "admin off", "import /run/jobcron/caddy/proxy-header"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing Caddy protection %s", want)
		}
	}
}
