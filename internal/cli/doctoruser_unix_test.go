//go:build unix

package cli

import (
	"os"
	"os/user"
	"strconv"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveTargetUserResolvesARealAccount covers the branches of
// resolveTargetUser that go to the user database and come back with somebody:
// --user naming the caller, --user naming anyone else, and the SUDO_UID a sudo
// invocation leaves behind.
//
// They need this system to name an account by a number. Windows names one by a
// SID — user.Current().Uid there is `S-1-5-21-…`, which lookupUser cannot make
// a uid of — and the whole of --user is a question about a uid: whose files to
// read, and whether that is somebody other than the caller. What that question
// even means on a system with no uids is unsettled, and the branches that do
// not consult the database at all stay in main_test.go, where they run
// everywhere.
func TestResolveTargetUserResolvesARealAccount(t *testing.T) {
	self, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	selfUID := os.Getuid()

	t.Run("--user names the invoking user: still self", func(t *testing.T) {
		got, err := resolveTargetUser(self.Username, paths.Env{UID: selfUID})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the caller's own uid")
		assert.Empty(t, got.Source, "naming yourself is not going cross-user")
	})

	t.Run("--user names someone else: cross-user, regardless of who's actually invoking", func(t *testing.T) {
		// selfEnv.UID is deliberately a uid nothing resolves to, so this exercises
		// the "different from invoker" branch without depending on whether the test
		// process happens to be root.
		got, err := resolveTargetUser(self.Username, paths.Env{UID: -1})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the uid of the user named")
		assert.NotEmpty(t, got.Source, "a target that is not the caller must say how it was arrived at")
	})

	t.Run("SUDO_UID auto-detected only when invoking as root", func(t *testing.T) {
		if selfUID == 0 {
			// The test process itself is root (e.g. a container test run), so
			// there's no non-root uid left to fake as SUDO_UID: a real sudo
			// invocation never sets SUDO_UID=0, and resolveTargetUser correctly
			// treats a lookup that resolves back to uid 0 as "no cross-user
			// target", the very thing this subtest exists to rule out.
			t.Skip("test process is already root: can't fake a distinct non-root SUDO_UID")
		}
		t.Setenv("SUDO_UID", strconv.Itoa(selfUID))
		got, err := resolveTargetUser("", paths.Env{UID: 0})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the uid sudo recorded")
		assert.NotEmpty(t, got.Source, "a target arrived at through SUDO_UID must say so")
	})
}
