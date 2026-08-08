package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManagedDaemonConnectedRequiresReadyControlChannel(t *testing.T) {
	ready := false
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if ready {
			w.Write([]byte(`{"control_connected":true}`))
			return
		}
		w.Write([]byte(`{"control_connected":false}`))
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	defer server.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	t.Setenv("DAEMON_PORT", port)

	if managedDaemonConnected() {
		t.Fatal("Daemon reported connected before the remote control channel was ready")
	}
	ready = true
	if !managedDaemonConnected() {
		t.Fatal("Daemon did not report the ready remote control channel")
	}
}
