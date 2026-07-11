package scripts

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const statePrefix = "Preview state: "

func TestPreviewInteractiveScriptSyntax(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join(".."))
	cmd := exec.Command("sh", "-n", "scripts/preview-interactive.sh")
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sh -n scripts/preview-interactive.sh: %v\n%s", err, output)
	}
}

func TestPreviewInteractiveUsesIsolatedState(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}

	repoRoot := filepath.Clean(filepath.Join(".."))
	cmd := exec.Command("sh", "scripts/preview-interactive.sh", strconv.Itoa(port))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "JOBCRON_PREVIEW_KEEP=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("capture launcher stdout: %v", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("start launcher: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), statePrefix) {
				lines <- strings.TrimSpace(strings.TrimPrefix(scanner.Text(), statePrefix))
				return
			}
		}
		close(lines)
	}()

	var stateDir string
	select {
	case stateDir = <-lines:
		if stateDir == "" {
			t.Fatal("launcher exited without printing its preview state directory")
		}
	case <-time.After(60 * time.Second):
		t.Fatal("timed out waiting for launcher state directory")
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateDir) })

	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve real home: %v", err)
	}
	rel, err := filepath.Rel(realHome, stateDir)
	if err != nil {
		t.Fatalf("compare preview state with real home: %v", err)
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("preview state directory %q is inside real home %q", stateDir, realHome)
	}
	if info, err := os.Stat(stateDir); err != nil || !info.IsDir() {
		t.Fatalf("preview state directory %q was not created: %v", stateDir, err)
	}

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	previewURL := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, requestErr := client.Get(previewURL)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode < http.StatusInternalServerError {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("preview server never became reachable at %s: %v", previewURL, requestErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
