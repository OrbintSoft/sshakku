package secretservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	require.Error(t, err, "an unresponsive daemon must not be reported as a collection")
	assert.ErrorIs(t, err, context.DeadlineExceeded, "the error must say the deadline was exceeded")
	// The deadline is 150ms; anything near the test binary's default timeout
	// would mean the call was not bounded at all.
	assert.Less(t, elapsed, 5*time.Second, "the call was not bounded")
}

// TestClientPropertyRejectsMalformedName covers property's guard against a
// property name with no interface/member separator, the one branch the live
// Get/Items/Attributes tests never reach.
func TestClientPropertyRejectsMalformedName(t *testing.T) {
	var c Client
	_, err := c.property(nil, "no-dot-here")
	require.Error(t, err, "a malformed property name must be refused")
	assert.ErrorContains(t, err, "malformed property name", "the error must say what was wrong with it")
}
