//go:build windows

package reach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// countingProber records whether it was ever asked anything.
type countingProber struct{ asked int }

func (p *countingProber) Reachable(context.Context, string) bool {
	p.asked++
	return true
}

// The ownership question this type exists to ask cannot be asked here: it is
// given a uid, and this system answers who owns a thing with an account
// identifier and an access-control list instead. So it reports nothing
// reachable, whatever it is pointed at.
//
// What matters as much as the answer is that the wrapped prober is left alone.
// This type is used for cross-user diagnosis, where the point is to report what
// the *target* account could reach — consulting the wrapped prober would report
// what an elevated caller can reach past the gate, which is the opposite of the
// question, and it would arrive looking like a real answer.
func TestAGateThisSystemCannotApplyRefusesRatherThanBypassing(t *testing.T) {
	wrapped := &countingProber{}
	g := UIDGatedProber{UID: 1000, Prober: wrapped}

	for _, endpoint := range []string{"", `\\.\pipe\openssh-ssh-agent`, `\\.\pipe\anything-at-all`} {
		assert.Falsef(t, g.Reachable(t.Context(), endpoint),
			"a gate that cannot be applied answers no, including for %q", endpoint)
	}

	assert.Zero(t, wrapped.asked,
		"the wrapped prober is never consulted: its answer is the one this gate exists to withhold")
}

// UIDGatedProber must satisfy Prober.
var _ Prober = UIDGatedProber{}
