package main

import (
	"net"
	"testing"
)

func TestListenFallsBackWhenPortBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy a port: %v", err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	ln, addr, err := listen(busyPort)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	if addr == busy.Addr().String() {
		t.Errorf("listen returned the busy address %s; should have fallen back", addr)
	}
}
