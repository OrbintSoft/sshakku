//go:build windows

package launcher

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The snapshot is the one thing here no other machine can check, so this asks
// the real system for the real tree and looks for the process it already knows
// the answer about: this one.
func TestTheRealTreeHoldsTheProcessDoingTheAsking(t *testing.T) {
	src := NewToolhelpAncestry()

	ppid, name, ok := src.Parent(t.Context(), os.Getpid())

	require.True(t, ok, "a running process must be in the system's own table of running processes")
	assert.Equal(t, os.Getppid(), ppid, "and its parent must be the one it was really started by")
	assert.True(t, strings.HasSuffix(strings.ToLower(name), ".exe"),
		"the name is the image file's, which is what the labels below are written against; got %q", name)
	assert.NotContains(t, name, string(os.PathSeparator), "a snapshot names the file, not the path to it")
}

// A chain of one is not attribution. This walks up from the test binary and
// expects to arrive somewhere, which is the whole point of reading the tree.
func TestTheRealTreeCanBeWalkedUpwards(t *testing.T) {
	chain := Ancestry(t.Context(), os.Getpid(), NewToolhelpAncestry())

	require.GreaterOrEqual(t, len(chain), 2, "a process started by something must have that something above it")
	assert.Equal(t, os.Getpid(), chain[0].PID, "the walk starts at the process asked about")
	assert.Equal(t, os.Getppid(), chain[1].PID)
	for _, p := range chain {
		assert.NotEmpty(t, p.Name, "every step of a chain a user reads has to be named")
	}
	assert.NotEmpty(t, Chain(chain))
}

// The table is this platform's own answer about which names mean something, so
// it is checked where it lives.
func TestTheNamesThisSystemRecognises(t *testing.T) {
	// Every name the table answers for, alternates included: each one is a
	// sentence somebody reads in a report about their own machine, and a name
	// that quietly stopped being recognised reads as "an unknown launcher",
	// which is the answer for a program nobody has ever heard of.
	known := map[string]string{
		"explorer.exe":        "desktop shell",
		"winlogon.exe":        "interactive logon",
		"userinit.exe":        "interactive logon",
		"sshd.exe":            "sshd",
		"powershell.exe":      "Windows PowerShell",
		"pwsh.exe":            "PowerShell",
		"cmd.exe":             "cmd",
		"services.exe":        "service control manager",
		"svchost.exe":         "service host",
		"WindowsTerminal.exe": "terminal",
		"OpenConsole.exe":     "terminal",
		"conhost.exe":         "terminal",
		"bash.exe":            "bash.exe",
		"sh.exe":              "sh.exe",
		"zsh.exe":             "zsh.exe",
	}

	for image, expect := range known {
		label, ok := launcherLabel(image)
		assert.True(t, ok, "%s is a launcher this system should recognise", image)
		assert.Contains(t, label, expect)
	}

	// The case a file happens to have on disk is not a fact about the program.
	upper, ok := launcherLabel("EXPLORER.EXE")
	require.True(t, ok, "file names here are matched the way the system matches them")
	lower, _ := launcherLabel("explorer.exe")
	assert.Equal(t, lower, upper)

	_, ok = launcherLabel("ssh-agent.exe")
	assert.False(t, ok, "the agent is what is being attributed, not something that attributes")
	_, ok = launcherLabel("")
	assert.False(t, ok)
}

// There is no record here that outlives a process's parent and still names what
// started it, so a chain that dead-ends has nothing further to offer.
func TestNothingSurvivesAParentThatIsGone(t *testing.T) {
	assert.Equal(t, "an unknown launcher", reparentedLabel(""))
	assert.Equal(t, "an unknown launcher", reparentedLabel("a cgroup this system does not have"))
}

// The attribution rule is shared; what it arrives at is this platform's answer,
// so it is asked here against this platform's names.
func TestStartedByOnWindows(t *testing.T) {
	t.Run("the nearest ancestor anybody recognises is the answer", func(t *testing.T) {
		chain := []ProcInfo{
			{PID: 900, Name: "ssh-agent.exe"},
			{PID: 800, Name: "some-wrapper.exe"},
			{PID: 700, Name: "explorer.exe"},
		}

		who, ok := StartedBy(chain, "")

		require.True(t, ok)
		assert.Equal(t, "the Windows desktop shell (explorer)", who,
			"and not the wrapper in between, which names nothing a user could act on")
	})

	t.Run("a chain nobody recognises still names the parent", func(t *testing.T) {
		chain := []ProcInfo{{PID: 900, Name: "ssh-agent.exe"}, {PID: 800, Name: "some-build-tool.exe"}}

		who, ok := StartedBy(chain, "")

		require.True(t, ok)
		assert.Equal(t, "some-build-tool.exe", who,
			"an unrecognised name is still better than nothing: it is where a person goes to look")
	})

	t.Run("an agent with nothing above it attributes nothing", func(t *testing.T) {
		_, ok := StartedBy([]ProcInfo{{PID: 900, Name: "ssh-agent.exe"}}, "")

		assert.False(t, ok, "a chain of one is the agent itself, which did not launch itself")
	})

	// The shared rule has a step for a process whose launcher is gone from the
	// tree, and that step asks this platform what else is known about it. Here
	// the answer is nothing, so what the rule arrives at is the unknown launcher
	// rather than a record this system does not keep. Asked here because it is
	// the meeting of the shared rule and this platform's empty answer, and
	// neither half says on its own what the pair produces.
	t.Run("a launcher gone from the tree is named as unknown and not invented", func(t *testing.T) {
		chain := []ProcInfo{{PID: 900, Name: "ssh-agent.exe"}, {PID: 1, Name: "whatever-holds-pid-1.exe"}}

		who, ok := StartedBy(chain, "a record this system does not keep")

		require.True(t, ok, "there is still something to say, and it is said rather than withheld")
		assert.Equal(t, "an unknown launcher", who,
			"nothing outlives a parent here, so nothing is read back out of what was handed in")
	})
}

// This system groups processes in job objects, which a launcher opts into
// rather than something every process carries, so there is no record here that
// survives a parent and still names what started it. Reporting nothing is the
// whole truth, and reporting it for every process asked about is what keeps the
// attribution above honest.
func TestNoCgroupsReportsNothing(t *testing.T) {
	unit, ok := NoCgroups{}.Cgroup(os.Getpid())

	assert.False(t, ok, "there is no such record here, and saying so is the answer")
	assert.Empty(t, unit, "and nothing is handed back to be read as one")
}
