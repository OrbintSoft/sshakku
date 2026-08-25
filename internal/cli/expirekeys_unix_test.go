//go:build unix

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// Both tests below ask for a store that cannot be listed, and ask for it with a
// regular file where the directory belongs — which is a refusal only this kind
// of system produces. A system that opens a file as a directory and finds it
// empty has no refusal to give here: it reports a store with nothing in it,
// which is the very answer these exist to tell apart.

// A store that cannot be read is not a store with nothing in it: answering with
// an empty list would tell the expirer there is no key to take out, which is the
// same thing it hears about a machine that has none, and the key would stay in
// the agent past its time with nothing said.
func TestRecordsThatCannotBeReadAreNotNoKeysAtAll(t *testing.T) {
	unreadable := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(unreadable, []byte("x"), 0o600))

	_, err := keyRecords{store: keystate.Store{Dir: unreadable}}.Added()

	assert.Error(t, err, "the read failed, and the caller has to hear that rather than a count of zero")
}

// F53: the shell still opens. Expiring keys is something a session does on its
// way in, so what goes wrong here has one place to go — the log — and the
// session it is holding up is handed over either way.
func TestAShellOpensEvenWhenItsKeyRecordsCannotBeRead(t *testing.T) {
	tempRuntimeEnv(t)
	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir).WithSocketToken(paths.SocketToken())
	require.NoError(t, os.MkdirAll(filepath.Dir(keystateDir(layout)), 0o700))
	require.NoError(t, os.WriteFile(keystateDir(layout), []byte("not a directory"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(layout.LogFile), 0o700))

	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))
	d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}})
	d.runner, d.agentKeepsLifetimes = r, false

	var out, errOut bytes.Buffer
	require.Zerof(t, d.shellInit(t.Context(), &out, &errOut, nil),
		"a session must open whether or not its records could be read; stderr=%q", errOut.String())

	assert.Empty(t, r.Calls, "nothing is taken out of the agent on the strength of a list that was never read")
	logged, err := os.ReadFile(layout.LogFile)
	require.NoError(t, err, "the session log must have been written")
	assert.Contains(t, string(logged), "expire keys",
		"what went wrong goes to the log, which is the only place a shell opening can put it")
}
