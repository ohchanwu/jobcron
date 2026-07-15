package localdb

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type commandCall struct {
	name string
	args []string
}

type fakeCommandRunner struct {
	lookPathErr error
	run         func(context.Context, string, ...string) ([]byte, error)
	calls       []commandCall
}

func (f *fakeCommandRunner) LookPath(string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return "/usr/local/bin/docker", nil
}

func (f *fakeCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, commandCall{name: name, args: append([]string(nil), args...)})
	if f.run != nil {
		return f.run(ctx, name, args...)
	}
	return nil, nil
}

func availablePort(context.Context, string) (net.Listener, error) {
	return &fakeListener{}, nil
}

type fakeListener struct{}

func (*fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (*fakeListener) Close() error              { return nil }
func (*fakeListener) Addr() net.Addr            { return nil }

func occupiedPort(context.Context, string) (net.Listener, error) {
	return nil, errors.New("address already in use")
}

func reachablePostUpTCP(context.Context, string, string) (net.Conn, error) {
	return &fakeConn{}, nil
}

type fakeConn struct{}

func (*fakeConn) Read([]byte) (int, error)         { return 0, errors.New("not implemented") }
func (*fakeConn) Write([]byte) (int, error)        { return 0, errors.New("not implemented") }
func (*fakeConn) Close() error                     { return nil }
func (*fakeConn) LocalAddr() net.Addr              { return nil }
func (*fakeConn) RemoteAddr() net.Addr             { return nil }
func (*fakeConn) SetDeadline(time.Time) error      { return nil }
func (*fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (*fakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestEnsureUsesFixedProjectAndEmbeddedCompose(t *testing.T) {
	runner := &fakeCommandRunner{}
	runner.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if !contains(args, "up") {
			return nil, nil
		}

		want := []string{"compose", "-p", ProjectName, "-f"}
		if len(args) != 10 || !reflect.DeepEqual(args[:4], want) ||
			!reflect.DeepEqual(args[5:], []string{"up", "-d", "--wait", "--wait-timeout", "60"}) {
			t.Errorf("compose startup args = %q", args)
		}
		composePath := args[4]
		got, err := os.ReadFile(composePath)
		if err != nil {
			t.Fatalf("read materialized Compose file: %v", err)
		}
		if !reflect.DeepEqual(got, ComposeYAML) {
			t.Error("materialized Compose definition differs from embedded bytes")
		}
		if info, err := os.Stat(filepath.Dir(composePath)); err != nil {
			t.Fatalf("stat private temporary directory: %v", err)
		} else if info.Mode().Perm() != 0o700 {
			t.Errorf("temporary directory mode = %o, want 700", info.Mode().Perm())
		}
		return nil, nil
	}

	got, err := ensure(context.Background(), runner, availablePort, reachablePostUpTCP)
	if err != nil {
		t.Fatalf("ensure local database: %v", err)
	}
	if got != DatabaseURL {
		t.Errorf("URL = %q, want %q", got, DatabaseURL)
	}
}

func TestEnsureReturnsURLOnlyAfterComposeWaitSucceeds(t *testing.T) {
	runner := &fakeCommandRunner{run: failCommandContaining("up", errors.New("exit status 1"))}

	got, err := ensure(context.Background(), runner, availablePort, reachablePostUpTCP)
	if got != "" {
		t.Errorf("URL = %q before Compose wait succeeded", got)
	}
	assertEnsureError(t, err, "Docker Compose health wait")
}

func TestEnsureExplainsMissingDockerAndDatabaseURLBypass(t *testing.T) {
	runner := &fakeCommandRunner{lookPathErr: errors.New("executable file not found")}

	got, err := ensure(context.Background(), runner, availablePort, reachablePostUpTCP)
	if got != "" {
		t.Errorf("URL = %q with Docker missing", got)
	}
	assertEnsureError(t, err, "Docker executable")
	if len(runner.calls) != 0 {
		t.Errorf("commands ran with Docker missing: %#v", runner.calls)
	}
}

func TestEnsureExplainsMissingComposePlugin(t *testing.T) {
	runner := &fakeCommandRunner{run: failCommandContaining("version", errors.New("unknown command compose"))}

	_, err := ensure(context.Background(), runner, availablePort, reachablePostUpTCP)
	assertEnsureError(t, err, "Docker Compose plugin")
}

func TestEnsureExplainsDaemonFailure(t *testing.T) {
	runner := &fakeCommandRunner{run: failCommandContaining("info", errors.New("cannot connect"))}

	_, err := ensure(context.Background(), runner, availablePort, reachablePostUpTCP)
	assertEnsureError(t, err, "Docker daemon")
}

func TestEnsureRefusesForeignPortOwner(t *testing.T) {
	runner := &fakeCommandRunner{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if contains(args, "ps") {
			return []byte("someone-elses-project\tpostgres\n"), nil
		}
		return nil, nil
	}}

	_, err := ensure(context.Background(), runner, occupiedPort, reachablePostUpTCP)
	assertEnsureError(t, err, "foreign listener on port 55432")
	for _, call := range runner.calls {
		if contains(call.args, "up") {
			t.Errorf("startup ran despite foreign listener: %q", call.args)
		}
	}
}

func TestEnsurePreservesContainerStateOnHealthTimeout(t *testing.T) {
	runner := &fakeCommandRunner{run: failCommandContaining("up", errors.New("health timeout"))}

	_, err := ensure(context.Background(), runner, availablePort, reachablePostUpTCP)
	assertEnsureError(t, err, "Docker Compose health wait")
	message := err.Error()
	for _, phrase := range []string{
		"docker compose -p " + ProjectName + " -f ",
		" ps",
		" logs postgres",
		"service state was preserved",
	} {
		if !strings.Contains(message, phrase) {
			t.Errorf("error %q does not include diagnostic %q", message, phrase)
		}
	}
	for _, call := range runner.calls {
		for _, destructive := range []string{"down", "rm", "remove", "volume"} {
			if contains(call.args, destructive) {
				t.Errorf("destructive command ran after health timeout: %q", call.args)
			}
		}
	}
}

func TestEnsureHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeCommandRunner{}

	_, err := ensure(ctx, runner, availablePort, reachablePostUpTCP)
	assertEnsureError(t, err, "caller cancellation")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("commands ran after caller cancellation: %#v", runner.calls)
	}
}

func TestManagedLocalStartupRequiresHostTCPReachability(t *testing.T) {
	runner := &fakeCommandRunner{}
	dialed := false
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		dialed = true
		if len(runner.calls) == 0 || !contains(runner.calls[len(runner.calls)-1].args, "up") {
			t.Errorf("post-up TCP probe ran before Compose wait: %#v", runner.calls)
		}
		if network != "tcp" {
			t.Errorf("network = %q, want tcp", network)
		}
		if address != localAddress {
			t.Errorf("address = %q, want %q", address, localAddress)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("post-up TCP probe context has no deadline")
		} else if remaining := time.Until(deadline); remaining <= 0 || remaining > 3*time.Second {
			t.Errorf("post-up TCP probe deadline remaining = %v, want (0, 3s]", remaining)
		}
		return &fakeConn{}, nil
	}

	got, err := ensure(context.Background(), runner, availablePort, dial)
	if err != nil {
		t.Fatalf("ensure local database: %v", err)
	}
	if !dialed {
		t.Fatal("post-up TCP reachability was not checked")
	}
	if got != DatabaseURL {
		t.Errorf("URL = %q, want %q", got, DatabaseURL)
	}
}

func TestEnsureRejectsRefusedPostUpTCPAndPreservesState(t *testing.T) {
	runner := &fakeCommandRunner{}
	dial := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("connect: connection refused")
	}

	got, err := ensure(context.Background(), runner, availablePort, dial)
	if got != "" {
		t.Errorf("URL = %q despite unreachable host TCP", got)
	}
	assertEnsureError(t, err, "TCP reachability")
	for _, phrase := range []string{"127.0.0.1:55432", "port 55432", "connection refused", "service state was preserved", " ps", " logs postgres"} {
		if !strings.Contains(err.Error(), phrase) {
			t.Errorf("error %q does not contain %q", err, phrase)
		}
	}
	for _, call := range runner.calls {
		for _, destructive := range []string{"down", "rm", "remove", "volume"} {
			if contains(call.args, destructive) {
				t.Errorf("destructive command ran after TCP failure: %q", call.args)
			}
		}
	}
}

func TestEnsureHonorsCallerCancellationDuringPostUpTCPProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeCommandRunner{}
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		cancel()
		return nil, ctx.Err()
	}

	_, err := ensure(ctx, runner, availablePort, dial)
	assertEnsureError(t, err, "TCP reachability")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	for _, phrase := range []string{"service state was preserved", " ps", " logs postgres"} {
		if !strings.Contains(err.Error(), phrase) {
			t.Errorf("error %q does not contain %q", err, phrase)
		}
	}
}

func TestEnsurePreservesDiagnosticsWhenCanceledImmediatelyAfterComposeWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var composePath string
	runner := &fakeCommandRunner{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if contains(args, "up") {
			composePath = args[4]
			cancel()
		}
		return nil, nil
	}}
	dial := func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("TCP dial ran after caller cancellation")
		return nil, nil
	}

	_, err := ensure(ctx, runner, availablePort, dial)
	assertEnsureError(t, err, "caller cancellation")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	for _, phrase := range []string{
		"service state was preserved",
		"docker compose -p " + ProjectName + " -f " + composePath + " ps",
		"docker compose -p " + ProjectName + " -f " + composePath + " logs postgres",
	} {
		if !strings.Contains(err.Error(), phrase) {
			t.Errorf("error %q does not contain %q", err, phrase)
		}
	}
	if composePath == "" {
		t.Fatal("Compose path was not captured")
	}
	defer os.RemoveAll(filepath.Dir(composePath))
	if _, statErr := os.Stat(composePath); statErr != nil {
		t.Errorf("retained Compose file: %v", statErr)
	}
}

func assertEnsureError(t *testing.T, err error, component string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", component)
	}
	for _, phrase := range []string{component, "DATABASE_URL"} {
		if !strings.Contains(err.Error(), phrase) {
			t.Errorf("error %q does not contain %q", err, phrase)
		}
	}
}

func failCommandContaining(argument string, failure error) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if contains(args, argument) {
			return nil, failure
		}
		return nil, nil
	}
}

func contains(arguments []string, want string) bool {
	for _, argument := range arguments {
		if argument == want {
			return true
		}
	}
	return false
}
