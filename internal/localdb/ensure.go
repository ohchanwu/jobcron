package localdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ProjectName = "jobcron-local"
	DatabaseURL = "postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable"

	localAddress    = "127.0.0.1:55432"
	tcpProbeTimeout = 2 * time.Second
)

type commandRunner interface {
	LookPath(string) (string, error)
	Run(context.Context, string, ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type listenFunc func(context.Context, string) (net.Listener, error)
type dialFunc func(context.Context, string, string) (net.Conn, error)

// Ensure starts the managed local PostgreSQL service and waits until it is
// healthy. Callers with an explicit DATABASE_URL should bypass Ensure.
func Ensure(ctx context.Context) (string, error) {
	return ensure(ctx, execCommandRunner{}, listenLocalPort, dialLocalTCP)
}

func ensure(ctx context.Context, runner commandRunner, listen listenFunc, dial dialFunc) (string, error) {
	if err := cancellationError(ctx); err != nil {
		return "", err
	}

	docker, err := runner.LookPath("docker")
	if err != nil {
		return "", componentError("Docker executable", err)
	}
	if _, err := runner.Run(ctx, docker, "compose", "version"); err != nil {
		return "", commandError(ctx, "Docker Compose plugin", err)
	}
	if _, err := runner.Run(ctx, docker, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return "", commandError(ctx, "Docker daemon", err)
	}

	listener, listenErr := listen(ctx, localAddress)
	if listenErr == nil {
		if err := listener.Close(); err != nil {
			return "", componentError("local port 55432 probe", err)
		}
	} else {
		owner, err := runner.Run(ctx, docker,
			"ps",
			"--filter", "publish=55432",
			"--format", `{{.Label "com.docker.compose.project"}}\t{{.Label "com.docker.compose.service"}}`,
		)
		if err != nil {
			return "", commandError(ctx, "port 55432 owner inspection", err)
		}
		if !managedPostgresOwnsPort(string(owner)) {
			return "", componentError("foreign listener on port 55432", listenErr)
		}
	}

	if err := cancellationError(ctx); err != nil {
		return "", err
	}
	tempDir, err := os.MkdirTemp("", ProjectName+"-*")
	if err != nil {
		return "", componentError("private Compose directory", err)
	}
	keepTempDir := false
	defer func() {
		if !keepTempDir {
			_ = os.RemoveAll(tempDir)
		}
	}()

	composePath := filepath.Join(tempDir, "compose.yaml")
	if err := os.WriteFile(composePath, ComposeYAML, 0o600); err != nil {
		return "", componentError("embedded Compose materialization", err)
	}

	_, err = runner.Run(ctx, docker,
		"compose", "-p", ProjectName, "-f", composePath,
		"up", "-d", "--wait", "--wait-timeout", "60",
	)
	if err != nil {
		keepTempDir = true
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", composeFailure(composePath, componentError("caller cancellation", ctxErr))
		}
		return "", composeFailure(composePath, err)
	}
	if err := cancellationError(ctx); err != nil {
		keepTempDir = true
		return "", composeFailure(composePath, err)
	}
	probeCtx, cancelProbe := context.WithTimeout(ctx, tcpProbeTimeout)
	connection, err := dial(probeCtx, "tcp", localAddress)
	cancelProbe()
	if err != nil {
		keepTempDir = true
		return "", composeFailure(composePath,
			componentError("TCP reachability probe for 127.0.0.1:55432 on port 55432", err))
	}
	_ = connection.Close()

	return DatabaseURL, nil
}

func listenLocalPort(ctx context.Context, address string) (net.Listener, error) {
	var config net.ListenConfig
	return config.Listen(ctx, "tcp", address)
}

func dialLocalTCP(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

func managedPostgresOwnsPort(output string) bool {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[0] == ProjectName && fields[1] == "postgres" {
			return true
		}
	}
	return false
}

func cancellationError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return componentError("caller cancellation", err)
	}
	return nil
}

func commandError(ctx context.Context, component string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return componentError("caller cancellation", ctxErr)
	}
	return componentError(component, err)
}

func componentError(component string, err error) error {
	return fmt.Errorf("%s failed: %w; set DATABASE_URL to bypass managed local PostgreSQL startup", component, err)
}

func composeFailure(composePath string, err error) error {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		err = componentError("Docker Compose health wait", err)
	}
	return fmt.Errorf("service state was preserved. Diagnose with:\n  docker compose -p %s -f %s ps\n  docker compose -p %s -f %s logs postgres\nFailure: %w",
		ProjectName, composePath, ProjectName, composePath, err)
}
