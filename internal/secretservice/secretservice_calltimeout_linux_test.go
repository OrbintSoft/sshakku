package secretservice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestClientCallTimesOutWhenDaemonUnresponsive proves the bound the ordinary
// D-Bus round-trips gained: against a daemon that stays connected but never
// answers (fakeService.hang), a call must return a deadline error promptly
// rather than block the caller forever, which a plain flags-only call would.
func TestClientCallTimesOutWhenDaemonUnresponsive(t *testing.T) {
	client, svc := newTestClient(t, "")
	client.CallTimeout = 150 * time.Millisecond

	release := make(chan struct{})
	t.Cleanup(func() { close(release) }) // let the blocked handler unwind after we give up
	svc.hangOn(release)

	start := time.Now()
	_, err := client.Collection("sshakku", "SSHakku")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Collection succeeded against an unresponsive daemon; want a deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Collection error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	// The deadline is 150ms; anything near the test binary's default timeout
	// would mean the call was not bounded at all.
	if elapsed > 5*time.Second {
		t.Fatalf("Collection took %s to fail; the call was not bounded", elapsed)
	}
}

// TestClientPropertyRejectsMalformedName covers property's guard against a
// property name with no interface/member separator, the one branch the live
// Get/Items/Attributes tests never reach.
func TestClientPropertyRejectsMalformedName(t *testing.T) {
	var c Client
	_, err := c.property(nil, "no-dot-here")
	if err == nil {
		t.Fatal("property accepted a malformed name; want an error")
	}
	if !strings.Contains(err.Error(), "malformed property name") {
		t.Fatalf("property error = %v, want it to mention a malformed property name", err)
	}
}
