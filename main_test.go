package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestBuildSubscriberClientID verifies that the generated client_id contains
// the base name, hostname, and current PID — preventing the "session taken over"
// MQTT loop caused by a fixed client_id when multiple daemons run concurrently.
func TestBuildSubscriberClientID(t *testing.T) {
	base := "claude-mesh"
	id := buildSubscriberClientID(base)

	hostname, _ := os.Hostname()
	pid := strconv.Itoa(os.Getpid())

	if !strings.Contains(id, base+"-sub-") {
		t.Errorf("client_id %q does not contain base prefix %q", id, base+"-sub-")
	}
	if hostname != "" && !strings.Contains(id, hostname) {
		t.Errorf("client_id %q does not contain hostname %q", id, hostname)
	}
	if !strings.Contains(id, pid) {
		t.Errorf("client_id %q does not contain pid %q", id, pid)
	}
	// Sanity: format is base-sub-hostname-pid
	expected := fmt.Sprintf("%s-sub-%s-%s", base, hostname, pid)
	if id != expected {
		t.Errorf("client_id = %q, want %q", id, expected)
	}
}
