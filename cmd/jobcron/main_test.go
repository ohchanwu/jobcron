package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohchanwu/jobcron/internal/appdata"
	"github.com/ohchanwu/jobcron/internal/config"
	"github.com/ohchanwu/jobcron/internal/credential"
	"github.com/ohchanwu/jobcron/internal/localdb"
)

type fakeRuntimeUserStore struct {
	managedID    int64
	managedErr   error
	soleID       int64
	soleOK       bool
	soleErr      error
	managedCalls int
	soleCalls    int
}

func (s *fakeRuntimeUserStore) EnsureManagedLocalOwner(context.Context) (int64, error) {
	s.managedCalls++
	return s.managedID, s.managedErr
}

func (s *fakeRuntimeUserStore) SoleOwnerUserID(context.Context) (int64, bool, error) {
	s.soleCalls++
	return s.soleID, s.soleOK, s.soleErr
}

func (s *fakeRuntimeUserStore) Close() error { return nil }

func stubRuntimeDependencies(
	t *testing.T,
	ensure func(context.Context) (string, error),
	loadKey func() ([]byte, error),
	openStore func(string) (runtimeUserStore, error),
) {
	t.Helper()
	oldEnsure := ensureLocalPostgres
	oldLoadKey := loadLocalMasterKey
	oldOpenStore := openRuntimeUserStore
	ensureLocalPostgres = ensure
	loadLocalMasterKey = loadKey
	openRuntimeUserStore = openStore
	t.Cleanup(func() {
		ensureLocalPostgres = oldEnsure
		loadLocalMasterKey = oldLoadKey
		openRuntimeUserStore = oldOpenStore
	})
}

func TestResolveRuntimeExplicitURLBypassesLocalEnsure(t *testing.T) {
	const explicitURL = "postgres://explicit.example.invalid/jobcron"
	ensureCalls := 0
	openedURL := ""
	store := &fakeRuntimeUserStore{soleID: 41, soleOK: true}
	stubRuntimeDependencies(t,
		func(context.Context) (string, error) {
			ensureCalls++
			return "", errors.New("managed local startup must be bypassed")
		},
		func() ([]byte, error) { return bytes.Repeat([]byte{0x11}, credential.MasterKeyBytes), nil },
		func(databaseURL string) (runtimeUserStore, error) {
			openedURL = databaseURL
			return store, nil
		},
	)

	runtime, err := resolvePostgresRuntime(context.Background(), config.Config{DatabaseURL: explicitURL})
	if err != nil {
		t.Fatalf("resolvePostgresRuntime: %v", err)
	}
	if ensureCalls != 0 {
		t.Fatalf("local Ensure calls = %d, want 0", ensureCalls)
	}
	if openedURL != explicitURL || runtime.DatabaseURL != explicitURL {
		t.Fatalf("selected URLs = opened %q runtime %q, want explicit URL", openedURL, runtime.DatabaseURL)
	}
	if runtime.ManagedLocal {
		t.Fatal("ManagedLocal = true for explicit URL")
	}
	if runtime.UserID != 41 {
		t.Fatalf("UserID = %d, want 41", runtime.UserID)
	}
}

func TestResolveRuntimeProductionRequiresExplicitURL(t *testing.T) {
	ensureCalls, keyCalls, openCalls := 0, 0, 0
	stubRuntimeDependencies(t,
		func(context.Context) (string, error) { ensureCalls++; return "", nil },
		func() ([]byte, error) { keyCalls++; return nil, nil },
		func(string) (runtimeUserStore, error) { openCalls++; return nil, nil },
	)

	_, err := resolvePostgresRuntime(context.Background(), config.Config{Production: true})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("resolvePostgresRuntime error = %v, want DATABASE_URL requirement", err)
	}
	if ensureCalls != 0 || keyCalls != 0 || openCalls != 0 {
		t.Fatalf("dependency calls = ensure %d key %d open %d, want all zero", ensureCalls, keyCalls, openCalls)
	}
}

func TestResolveRuntimeLocalWithoutURLUsesManagedPostgres(t *testing.T) {
	const managedURL = localdb.DatabaseURL
	ensureCalls := 0
	store := &fakeRuntimeUserStore{managedID: 52}
	stubRuntimeDependencies(t,
		func(context.Context) (string, error) { ensureCalls++; return managedURL, nil },
		func() ([]byte, error) { return bytes.Repeat([]byte{0x22}, credential.MasterKeyBytes), nil },
		func(databaseURL string) (runtimeUserStore, error) {
			if databaseURL != managedURL {
				t.Fatalf("opened URL = %q, want managed URL", databaseURL)
			}
			return store, nil
		},
	)

	runtime, err := resolvePostgresRuntime(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("resolvePostgresRuntime: %v", err)
	}
	if ensureCalls != 1 || !runtime.ManagedLocal || runtime.DatabaseURL != managedURL {
		t.Fatalf("runtime URL = %q managed = %v ensure calls = %d, want managed URL and one Ensure",
			runtime.DatabaseURL, runtime.ManagedLocal, ensureCalls)
	}
	if store.managedCalls != 1 || store.soleCalls != 0 {
		t.Fatalf("owner calls = managed %d sole %d, want 1 and 0", store.managedCalls, store.soleCalls)
	}
}

func TestResolveRuntimeManagedOwnerIsLimitedToFixedDatabase(t *testing.T) {
	openCalls := 0
	stubRuntimeDependencies(t,
		func(context.Context) (string, error) { return "postgres://foreign.invalid/jobcron", nil },
		func() ([]byte, error) { return bytes.Repeat([]byte{0x23}, credential.MasterKeyBytes), nil },
		func(string) (runtimeUserStore, error) {
			openCalls++
			return &fakeRuntimeUserStore{managedID: 53}, nil
		},
	)

	_, err := resolvePostgresRuntime(context.Background(), config.Config{})
	if err == nil || !strings.Contains(err.Error(), "unexpected database URL") {
		t.Fatalf("resolvePostgresRuntime error = %v, want fixed-database refusal", err)
	}
	if openCalls != 0 {
		t.Fatalf("runtime store open calls = %d, want 0", openCalls)
	}
}

func TestResolveRuntimeLocalLoadsProtectedMasterKey(t *testing.T) {
	keyCalls := 0
	stubRuntimeDependencies(t,
		func(context.Context) (string, error) { return localdb.DatabaseURL, nil },
		func() ([]byte, error) {
			keyCalls++
			return bytes.Repeat([]byte{0x33}, credential.MasterKeyBytes), nil
		},
		func(string) (runtimeUserStore, error) { return &fakeRuntimeUserStore{managedID: 63}, nil },
	)

	runtime, err := resolvePostgresRuntime(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("resolvePostgresRuntime: %v", err)
	}
	if keyCalls != 1 {
		t.Fatalf("local master-key loads = %d, want 1", keyCalls)
	}
	if len(runtime.CredentialEncryptionKey) != credential.MasterKeyBytes {
		t.Fatalf("master-key length = %d, want %d", len(runtime.CredentialEncryptionKey), credential.MasterKeyBytes)
	}
}

func TestResolveRuntimeProductionNeverCreatesLocalMasterKey(t *testing.T) {
	configuredKey := bytes.Repeat([]byte{0x44}, credential.MasterKeyBytes)
	keyCalls := 0
	stubRuntimeDependencies(t,
		func(context.Context) (string, error) { return "", errors.New("unexpected Ensure") },
		func() ([]byte, error) { keyCalls++; return nil, errors.New("unexpected local key load") },
		func(string) (runtimeUserStore, error) {
			return nil, errors.New("production must not resolve a local user")
		},
	)

	runtime, err := resolvePostgresRuntime(context.Background(), config.Config{
		Production:              true,
		DatabaseURL:             "postgres://production.example.invalid/jobcron",
		CredentialEncryptionKey: configuredKey,
	})
	if err != nil {
		t.Fatalf("resolvePostgresRuntime: %v", err)
	}
	if keyCalls != 0 {
		t.Fatalf("local master-key loads = %d, want 0", keyCalls)
	}
	if len(runtime.CredentialEncryptionKey) != credential.MasterKeyBytes {
		t.Fatalf("master-key length = %d, want %d", len(runtime.CredentialEncryptionKey), credential.MasterKeyBytes)
	}
	if runtime.UserID != 0 || runtime.ManagedLocal {
		t.Fatalf("production runtime user ID = %d managed = %v, want authenticated user resolution",
			runtime.UserID, runtime.ManagedLocal)
	}
}

func TestResolveRuntimeManagedLocalCreatesStablePositiveUser(t *testing.T) {
	store := &fakeRuntimeUserStore{managedID: 74}
	stubRuntimeDependencies(t,
		func(context.Context) (string, error) { return localdb.DatabaseURL, nil },
		func() ([]byte, error) { return bytes.Repeat([]byte{0x55}, credential.MasterKeyBytes), nil },
		func(string) (runtimeUserStore, error) { return store, nil },
	)

	first, err := resolvePostgresRuntime(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("first resolvePostgresRuntime: %v", err)
	}
	second, err := resolvePostgresRuntime(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("second resolvePostgresRuntime: %v", err)
	}
	if first.UserID <= 0 || second.UserID != first.UserID {
		t.Fatalf("managed user IDs = %d, %d; want stable positive ID", first.UserID, second.UserID)
	}
	if store.managedCalls != 2 || store.soleCalls != 0 {
		t.Fatalf("owner calls = managed %d sole %d, want 2 and 0", store.managedCalls, store.soleCalls)
	}
}

func TestResolveRuntimeExplicitURLRequiresSolePositiveUser(t *testing.T) {
	tests := []struct {
		name    string
		store   *fakeRuntimeUserStore
		wantErr string
	}{
		{name: "zero users", store: &fakeRuntimeUserStore{}, wantErr: "exactly one existing user"},
		{name: "zero ID", store: &fakeRuntimeUserStore{soleID: 0, soleOK: true}, wantErr: "positive user ID"},
		{name: "multiple users", store: &fakeRuntimeUserStore{soleErr: errors.New("multiple users exist")}, wantErr: "multiple users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubRuntimeDependencies(t,
				func(context.Context) (string, error) { return "", errors.New("unexpected Ensure") },
				func() ([]byte, error) { return bytes.Repeat([]byte{0x66}, credential.MasterKeyBytes), nil },
				func(string) (runtimeUserStore, error) { return tt.store, nil },
			)
			_, err := resolvePostgresRuntime(context.Background(), config.Config{
				DatabaseURL: "postgres://explicit.example.invalid/jobcron",
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("resolvePostgresRuntime error = %v, want %q", err, tt.wantErr)
			}
			if tt.store.managedCalls != 0 || tt.store.soleCalls != 1 {
				t.Fatalf("owner calls = managed %d sole %d, want 0 and 1", tt.store.managedCalls, tt.store.soleCalls)
			}
		})
	}
}

func TestListenFallsBackWhenPortBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy a port: %v", err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	ln, addr, err := listen("127.0.0.1", busyPort)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	if addr == busy.Addr().String() {
		t.Errorf("listen returned the busy address %s; should have fallen back", addr)
	}
}

func TestListenUsesConfiguredHost(t *testing.T) {
	ln, addr, err := listen("0.0.0.0", 0)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "0.0.0.0" {
		t.Fatalf("host = %q, want 0.0.0.0", host)
	}
}

func TestOpenConfiguredStoreProductionUsesPostgresPath(t *testing.T) {
	_, err := openConfiguredStore(config.Config{
		Production:  true,
		DatabaseURL: "://not-a-valid-postgres-url",
	})
	if err == nil {
		t.Fatal("openConfiguredStore succeeded with an invalid PostgreSQL URL")
	}
	if got, want := err.Error(), "storage: open postgres"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("openConfiguredStore error = %v, want PostgreSQL open path error", err)
	}
}

func TestOpenConfiguredStoreDatabaseURLUsesPostgresOutsideProduction(t *testing.T) {
	_, err := openConfiguredStore(config.Config{
		DatabaseURL: "://not-a-valid-postgres-url",
	})
	if err == nil {
		t.Fatal("openConfiguredStore succeeded with an invalid PostgreSQL URL")
	}
	if got, want := err.Error(), "storage: open postgres"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("openConfiguredStore error = %v, want PostgreSQL open path error", err)
	}
}

func TestConfiguredStoreCredentialCipherUsesConfiguredKey(t *testing.T) {
	masterKey := bytes.Repeat([]byte{0x31}, credential.MasterKeyBytes)
	cipher, err := credentialCipherForConfig(config.Config{CredentialEncryptionKey: masterKey})
	if err != nil {
		t.Fatalf("credentialCipherForConfig: %v", err)
	}
	ciphertext, nonce, version, err := cipher.Seal(7, "anthropic", "synthetic-configured-key")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	opened, err := cipher.Open(7, "anthropic", ciphertext, nonce, version)
	if err != nil || opened != "synthetic-configured-key" {
		t.Fatalf("configured cipher round trip opened=%v err=%v", opened == "synthetic-configured-key", err)
	}
}

func TestConfiguredStoreCredentialCipherRequiresProductionKey(t *testing.T) {
	if _, err := credentialCipherForConfig(config.Config{Production: true}); err == nil {
		t.Fatal("credentialCipherForConfig accepted production without a master key")
	}
}

func TestConfiguredStoreCredentialCipherCreatesProtectedLocalKey(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("APPDATA", filepath.Join(root, "AppData", "Roaming"))

	cipher, err := credentialCipherForConfig(config.Config{})
	if err != nil {
		t.Fatalf("credentialCipherForConfig: %v", err)
	}
	ciphertext, nonce, version, err := cipher.Seal(8, "anthropic", "synthetic-local-key")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := cipher.Open(8, "anthropic", ciphertext, nonce, version); err != nil {
		t.Fatalf("Open: %v", err)
	}
	path, err := credential.DefaultMasterKeyPath()
	if err != nil {
		t.Fatalf("DefaultMasterKeyPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat local master key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("local master key mode = %o, want 600", perm)
	}
}

func TestPrepareApplicationDataMigratesLegacyDirectory(t *testing.T) {
	root := t.TempDir()
	legacyFile := filepath.Join(appdata.LegacyDir(root), "jobs.db")
	if err := os.MkdirAll(filepath.Dir(legacyFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyFile, []byte("database"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := prepareApplicationData(root); err != nil {
		t.Fatalf("prepareApplicationData: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(appdata.Dir(root), "jobs.db"))
	if err != nil || string(got) != "database" {
		t.Fatalf("migrated database = %q, err = %v", got, err)
	}
}

func TestVersionDoesNotPrepareApplicationData(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("APPDATA", filepath.Join(root, "AppData", "Roaming"))

	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	legacyFile := filepath.Join(appdata.LegacyDir(configRoot), "nested", "jobs.db")
	if err := os.MkdirAll(filepath.Dir(legacyFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyFile, []byte("database"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"jobcron", "--version"}
	t.Cleanup(func() { os.Args = oldArgs })
	main()

	got, err := os.ReadFile(legacyFile)
	if err != nil || string(got) != "database" {
		t.Fatalf("legacy data after --version = %q, err = %v", got, err)
	}
	if _, err := os.Stat(appdata.Dir(configRoot)); !os.IsNotExist(err) {
		t.Fatalf("canonical path changed by --version: %v", err)
	}
}
