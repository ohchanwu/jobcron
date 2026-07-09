package main

import (
	"net"
	"strings"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/config"
)

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

func TestHasVersionFlagAcceptsBooleanTrueForms(t *testing.T) {
	tests := [][]string{
		{"--version"},
		{"-version"},
		{"--version=true"},
		{"-version=true"},
	}
	for _, args := range tests {
		if !hasVersionFlag(args) {
			t.Fatalf("hasVersionFlag(%v) = false, want true", args)
		}
	}
}

func TestHasVersionFlagIgnoresBooleanFalseForms(t *testing.T) {
	tests := [][]string{
		{"--version=false"},
		{"-version=false"},
		{"--port", "7777"},
	}
	for _, args := range tests {
		if hasVersionFlag(args) {
			t.Fatalf("hasVersionFlag(%v) = true, want false", args)
		}
	}
}

func TestOpenConfiguredStoreProductionReturnsTask2Error(t *testing.T) {
	_, err := openConfiguredStore(config.Config{
		Production:  true,
		DatabaseURL: "postgres://db.example.invalid/jobs",
	})
	if err == nil || !strings.Contains(err.Error(), "Task 2") {
		t.Fatalf("openConfiguredStore error = %v, want Task 2 postgres storage error", err)
	}
}
