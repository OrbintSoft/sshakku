//go:build unix

package keys

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSSHAddArgs(t *testing.T) {
	tests := []struct {
		name     string
		lifetime time.Duration
		want     []string
	}{
		{"no expiry", 0, []string{"/k"}},
		{"negative no expiry", -time.Minute, []string{"/k"}},
		{"sub-second no expiry", 500 * time.Millisecond, []string{"/k"}},
		{"one hour", time.Hour, []string{"-t", "3600", "/k"}},
		{"eight hours", 8 * time.Hour, []string{"-t", "28800", "/k"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equalf(t, tc.want, sshAddArgs(tc.lifetime, "/k"),
				"how long the agent is asked to keep the key, for a lifetime of %v", tc.lifetime)
		})
	}
}
