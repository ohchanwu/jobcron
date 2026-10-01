package scripts

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeSecretVerifierFailsClosed(t *testing.T) {
	f := newRuntimeFixture(t)
	if r := f.run(t, validRuntimeSecret(), "prepare"); r.err != nil {
		t.Fatal(r.output)
	}
	// Synthetic daemon replies; only metadata inspection is exercised, no container runs.
	metadata := `[{"Config":{"Env":["DATABASE_URL_FILE=/run/jobcron/secrets/DATABASE_URL","SESSION_SECRET_FILE=/run/jobcron/secrets/SESSION_SECRET","JOBCRON_CREDENTIAL_ENCRYPTION_KEY_FILE=/run/jobcron/secrets/JOBCRON_CREDENTIAL_ENCRYPTION_KEY","JOBCRON_SIGNUP_ACCESS_CODE_FILE=/run/jobcron/secrets/JOBCRON_SIGNUP_ACCESS_CODE","JOBCRON_PROXY_SECRET_FILE=/run/jobcron/secrets/JOBCRON_PROXY_SECRET"]},"Mounts":[{"Type":"bind","Source":"/run/jobcron/secrets","Destination":"/run/jobcron/secrets","RW":false}]}]`
	caddy := `[{"Config":{"Env":["AWS_EC2_METADATA_DISABLED=true"]},"HostConfig":{"ReadonlyRootfs":true,"Tmpfs":{"/config":"mode=0700","/data":"mode=0700","/tmp":"mode=0700"}},"Mounts":[{"Type":"bind","Source":"/run/jobcron/caddy","Destination":"/run/jobcron/caddy","RW":false}]}]`
	appPath := filepath.Join(f.root, "app.json")
	caddyPath := filepath.Join(f.root, "caddy.json")
	writeFile(t, appPath, metadata, 0600)
	writeFile(t, caddyPath, caddy, 0600)
	f.env = append(f.env, "FAKE_APP="+appPath, "FAKE_CADDY="+caddyPath)
	writeExecutable(t, filepath.Join(f.binDir, "docker"), `#!/bin/sh
case "$*" in
 *" ps -q app") printf app;;
 *" ps -q caddy") printf caddy;;
 "inspect app") cat "$FAKE_APP";;
 "inspect caddy") cat "$FAKE_CADDY";;
 *) exit 1;;
esac
`)
	if r := f.run(t, validRuntimeSecret(), "verify-secrets"); r.err != nil || r.output != "runtime_secret_metadata_safe=true\n" {
		t.Fatalf("valid metadata rejected: %v %s", r.err, r.output)
	}
	for _, bad := range []string{`[]`, `{}`, strings.Replace(metadata, "DATABASE_URL_FILE=", "DATABASE_URL=", 1), strings.Replace(metadata, `"Config":{`, `"leak":"`+runtimeSecretFields["SESSION_SECRET"]+`","Config":{`, 1)} {
		writeFile(t, appPath, bad, 0600)
		r := f.run(t, validRuntimeSecret(), "verify-secrets")
		if r.err == nil {
			t.Fatal("unsafe metadata accepted")
		}
		assertNoRuntimeSecret(t, r.output)
	}
	writeFile(t, appPath, metadata, 0600)
	for _, bad := range []string{
		strings.Replace(caddy, `"ReadonlyRootfs":true`, `"ReadonlyRootfs":false`, 1),
		strings.Replace(caddy, `"/config":"mode=0700"`, `"/persistent":"mode=0700"`, 1),
		strings.Replace(caddy, `"Type":"bind"`, `"Type":"volume"`, 1),
		strings.Replace(caddy, `"RW":false`, `"RW":true`, 1),
		strings.Replace(caddy, `"Config":{`, `"leak":"`+runtimeSecretFields["JOBCRON_PROXY_SECRET"]+`","Config":{`, 1),
	} {
		writeFile(t, caddyPath, bad, 0600)
		r := f.run(t, validRuntimeSecret(), "verify-secrets")
		if r.err == nil {
			t.Fatal("unsafe Caddy metadata accepted")
		}
		assertNoRuntimeSecret(t, r.output)
	}
}
