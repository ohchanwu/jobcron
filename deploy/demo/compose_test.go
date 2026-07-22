package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestRenderedDemoCommandStartsSQLiteApp(t *testing.T) {
	compose := exec.Command("docker", "compose", "-f", "compose.yaml", "config", "--format", "json")
	compose.Env = append(os.Environ(),
		"JOBCRON_IMAGE=example.invalid/jobcron:sha-0123456789ab",
		"JOBCRON_ADMIN_TOKEN=synthetic-admin-token",
		"JOBCRON_PROXY_SECRET=synthetic-proxy-secret",
	)
	rendered, err := compose.Output()
	if err != nil {
		t.Fatalf("render demo Compose: %v", err)
	}
	var config struct {
		Services map[string]struct {
			Command []string `json:"command"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rendered, &config); err != nil {
		t.Fatalf("parse rendered demo Compose: %v", err)
	}
	wantCommand := []string{"--no-open", "--demo", "--host", "0.0.0.0", "--db", "/data/jobs.db"}
	if got := config.Services["app"].Command; fmt.Sprint(got) != fmt.Sprint(wantCommand) {
		t.Fatalf("rendered app command = %q, want %q", got, wantCommand)
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "jobcron")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/jobcron")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build jobcron: %v\n%s", err, output)
	}
	port := reservePort(t)
	databasePath := filepath.Join(dir, "jobs.db")
	args := append([]string{}, wantCommand...)
	args = append(args,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--db", databasePath,
	)
	app := exec.Command(binary, args...)
	t.Setenv("JOBCRON_ENV", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JOBCRON_STRICT_PORT", "1")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	app.Env = os.Environ()
	var output bytes.Buffer
	app.Stdout, app.Stderr = &output, &output
	if err := app.Start(); err != nil {
		t.Fatalf("start rendered demo command: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- app.Wait() }()
	t.Cleanup(func() {
		if app.ProcessState == nil {
			_ = app.Process.Kill()
			<-done
		}
	})

	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	waitForDemo(t, address, done, &output)
	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("demo database was not created at configured path: %v", err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + address + "/")
	if err != nil {
		t.Fatalf("GET rendered demo: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET rendered demo status = %d, want 200", response.StatusCode)
	}

	if runtime.GOOS == "windows" {
		err = app.Process.Kill()
	} else {
		err = app.Process.Signal(os.Interrupt)
	}
	if err != nil {
		t.Fatalf("stop rendered demo: %v", err)
	}
	select {
	case err := <-done:
		if err != nil && runtime.GOOS != "windows" {
			t.Fatalf("rendered demo shutdown: %v\n%s", err, output.String())
		}
	case <-time.After(7 * time.Second):
		t.Fatalf("rendered demo did not stop cleanly\n%s", output.String())
	}
}

func reservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func waitForDemo(t *testing.T, address string, done <-chan error, output fmt.Stringer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("rendered demo exited before binding: %v\n%s", err, output.String())
		default:
		}
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("rendered demo did not bind %s\n%s", address, output.String())
}
