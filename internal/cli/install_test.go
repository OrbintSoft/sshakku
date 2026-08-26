package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/install"
)

// errThisProgramHasNoPath is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errThisProgramHasNoPath = errors.New("this program has no path")

// The install and uninstall commands, against what docs/FEATURES.md promises:
// F44 — one command wires one shell, you can say which, and what it did is
// something you can go and look at; F47 — the environment step is skipped when
// you say so, and said to have been.
//
// Every case here names --profile and --no-path, and that is not tidiness: the
// account running the suite has startup files and a stored environment of its
// own, and a test that wired those would change the machine it ran on.

// wired runs one command as a person would, through the real dependencies, and
// returns the exit code with everything it printed.
func wired(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := realDeps().run(t.Context(), &stdout, &stderr, args)
	return code, stdout.String(), stderr.String()
}

// reported reads a report back as the fields it names.
//
// What is promised is that a person can go and look at what was done, which
// means the report has to carry the paths themselves rather than mention that
// there were some.
func reported(t *testing.T, stdout string) map[string]string {
	t.Helper()
	fields := map[string]string{}
	for line := range strings.SplitSeq(stdout, "\n") {
		if !strings.HasPrefix(line, "  ") {
			continue
		}
		name, value, found := strings.Cut(strings.TrimSpace(line), "  ")
		if !found {
			continue
		}
		fields[name] = strings.TrimSpace(value)
	}
	return fields
}

// aStartupFile is a file for the install to wire, in a directory of the test's
// own. It does not exist yet: an install writes one where there was none.
func aStartupFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "startup")
}

// F44: the interpreter you name is the one wired, and the report names the
// three things you would go and look at — the shell, the file it wrote into,
// and the hook that file now runs.
func TestTheShellYouNameIsTheOneWiredAndTheReportSaysWhereToLook(t *testing.T) {
	exe, kind := aWiredShell(t)
	installInto(t, t.TempDir())
	profile := aStartupFile(t)

	code, stdout, stderr := wired(t, "install", "--shell-exe", exe, "--profile", profile, "--no-path")
	require.Equalf(t, 0, code, "install: %s%s", stdout, stderr)

	fields := reported(t, stdout)
	assert.Equal(t, kind, fields["shell"], "the report says which shell was wired")
	assert.Equal(t, exe, fields["interpreter"], "and which program that was")
	assert.Equal(t, profile, fields["into"], "and the file a person goes and opens")

	hook := fields["hook"]
	require.NotEmpty(t, hook, "the report must name the hook that file now runs")
	require.FileExists(t, hook)

	content, err := os.ReadFile(profile)
	require.NoError(t, err, "the file named must have been written")
	assert.Contains(t, string(content), filepath.Base(hook),
		"the file wired must run the hook the report named")
}

// F44: uninstall takes the same selectors, and what surrounded the wiring comes
// back as it was.
func TestUninstallingLeavesTheFileAsItWasFound(t *testing.T) {
	exe, _ := aWiredShell(t)
	installInto(t, t.TempDir())
	profile := aStartupFile(t)
	before := "# a line of the account's own\n"
	require.NoError(t, os.WriteFile(profile, []byte(before), 0o600))

	code, stdout, stderr := wired(t, "install", "--shell-exe", exe, "--profile", profile, "--no-path")
	require.Equalf(t, 0, code, "install: %s%s", stdout, stderr)
	after, err := os.ReadFile(profile)
	require.NoError(t, err)
	require.NotEqual(t, before, string(after), "there must be something to take back out")

	code, stdout, stderr = wired(t, "uninstall", "--shell-exe", exe, "--profile", profile, "--no-path")
	require.Equalf(t, 0, code, "uninstall: %s%s", stdout, stderr)

	back, err := os.ReadFile(profile)
	require.NoError(t, err)
	assert.Equal(t, before, string(back), "the surrounding file comes back byte for byte")

	hook := reported(t, stdout)["hook"]
	require.NotEmpty(t, hook, "the report says which hook it removed")
	assert.NoFileExists(t, hook, "and that hook is gone")
	assert.NoDirExists(t, filepath.Dir(hook),
		"as is the directory it was alone in — an empty directory of ours is a trace like any other")
}

// F19, F44: where the shell already reads a drop-in directory, the wiring is a
// file of SSHakku's own inside it rather than a block in somebody else's file —
// and the report says which of the two it wrote, since only one of them is a
// file you can delete outright.
func TestWhereTheShellReadsADropInDirectoryTheWiringIsAFileOfItsOwn(t *testing.T) {
	installInto(t, t.TempDir())
	profile := aStartupFile(t)
	require.NoError(t, os.MkdirAll(profile+".d", 0o700))

	code, stdout, stderr := wired(t, "install", "--shell=bash", "--profile", profile, "--no-path")
	require.Equalf(t, 0, code, "install: %s%s", stdout, stderr)

	dropIn := reported(t, stdout)["into"]
	require.NotEmpty(t, dropIn)
	assert.Equal(t, profile+".d", filepath.Dir(dropIn), "the wiring went into the directory the shell reads")
	assert.Contains(t, reported(t, stdout)["wrote"], "a file of its own")
	assert.FileExists(t, dropIn)
	assert.NoFileExists(t, profile, "and the shell's own startup file was left alone")

	code, stdout, stderr = wired(t, "uninstall", "--shell=bash", "--profile", profile, "--no-path")
	require.Equalf(t, 0, code, "uninstall: %s%s", stdout, stderr)
	assert.NoFileExists(t, dropIn, "and it is taken back out")
}

// F19: a startup file that only ever held our wiring is not left behind empty.
//
// The pair matters more than either half: the case above proves a file with
// content of its own survives with that content, and this one proves a file
// that had none does not survive at all. "Every trace" is both.
func TestAFileThatHeldNothingButTheWiringIsNotLeftBehind(t *testing.T) {
	exe, _ := aWiredShell(t)
	installInto(t, t.TempDir())
	profile := aStartupFile(t)
	require.NoFileExists(t, profile, "this file is the install's to create")

	code, stdout, stderr := wired(t, "install", "--shell-exe", exe, "--profile", profile, "--no-path")
	require.Equalf(t, 0, code, "install: %s%s", stdout, stderr)
	require.FileExists(t, profile)

	code, stdout, stderr = wired(t, "uninstall", "--shell-exe", exe, "--profile", profile, "--no-path")
	require.Equalf(t, 0, code, "uninstall: %s%s", stdout, stderr)

	assert.NoFileExists(t, profile,
		"a file whose whole content was ours has nothing left to preserve, and an empty one is a trace")
}

// F44: a program that is not a shell is refused by name rather than wired, and
// nothing is written for it.
func TestAProgramThatIsNotAShellIsRefusedRatherThanWired(t *testing.T) {
	installInto(t, t.TempDir())
	profile := aStartupFile(t)
	// This test binary: a real program on this machine, and not a shell.
	notAShell := os.Args[0]

	code, _, stderr := wired(t, "install", "--shell-exe", notAShell, "--profile", profile, "--no-path")

	assert.Equal(t, 1, code, "an install that cannot be done is a failure")
	assert.Contains(t, stderr, notAShell, "and says which program it would not wire")
	assert.NoFileExists(t, profile, "nothing is written for a shell that was refused")
}

// F44: what cannot be understood is a usage error, not something to pick a
// value for.
func TestAnInstallItCannotUnderstandIsAUsageErrorAndWritesNothing(t *testing.T) {
	// Nothing should reach the disk at all here. The redirection is what makes
	// that an assertion rather than an assumption: were one of these to be
	// understood after all, it would write where the test can see it and not
	// into the account running the suite.
	installInto(t, t.TempDir())

	for _, testCase := range []struct {
		name string
		args []string
	}{
		{"a flag that is not one", []string{"--bogus"}},
		{"a shell this system cannot wire", []string{"--shell=fish"}},
		{"a flag given no value", []string{"--profile"}},
		{"a scope that is neither", []string{"--scope=everyone"}},
		{"hosts that are neither", []string{"--hosts=some"}},
		{"a value given to a flag that takes none", []string{"--no-path=yes"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			for _, command := range []string{"install", "uninstall"} {
				code, stdout, stderr := wired(t, append([]string{command}, testCase.args...)...)

				assert.Equalf(t, 2, code, "%s %v", command, testCase.args)
				assert.NotEmptyf(t, stderr, "%s must say what it did not understand", command)
				assert.Emptyf(t, stdout, "and report nothing as done")
			}
		})
	}
}

// F47: the environment is left alone when you say so, and the report says the
// step was skipped rather than leaving you to wonder.
func TestTheStoredEnvironmentIsLeftAloneWhenYouSaySo(t *testing.T) {
	exe, _ := aWiredShell(t)
	installInto(t, t.TempDir())

	code, stdout, stderr := wired(t, "install", "--shell-exe", exe, "--profile", aStartupFile(t), "--no-path")
	require.Equalf(t, 0, code, "install: %s%s", stdout, stderr)

	assert.Contains(t, reported(t, stdout)["path"], "--no-path",
		"a step that was skipped is reported as skipped, naming what skipped it")
}

// F44: with nothing named, the shell is the one that ran the command — and
// where nothing that ran it is a shell this system can wire, that is said and
// nothing is written. Both are the promise; a third outcome is not.
//
// Which of the two happens depends on what started the test run, which is the
// point: this is the real process tree and no fake one, so the machine gets to
// answer.
func TestWithNoShellNamedTheOneThatRanTheCommandIsTheOneWired(t *testing.T) {
	installInto(t, t.TempDir())
	profile := aStartupFile(t)

	code, stdout, stderr := wired(t, "install", "--profile", profile, "--no-path")

	if code == 0 {
		assert.NotEmpty(t, reported(t, stdout)["shell"], "a wiring names the shell it was for")
		assert.FileExists(t, profile, "and the file it wired is there")
		return
	}
	assert.Contains(t, stderr, "--shell",
		"a process tree naming no shell must ask for one rather than pick one")
	assert.NoFileExists(t, profile, "and must write nothing")
}

// F47: what became of the step that makes SSHakku runnable by name is reported,
// and the three things it can have been are told apart. "Already there" and
// "there is nowhere to record one" are both a step that changed nothing, and only
// one of them is worth going to look at — so the words come from the system rather
// than from a guess about which it was.
func TestTheReportSaysWhatBecameOfTheSearchListStep(t *testing.T) {
	entry := filepath.Join(string(filepath.Separator), "opt", "sshakku", "bin")
	installer := wiring{name: "install"}
	remover := wiring{name: "uninstall"}

	assert.Equal(t, "not recorded (--no-path)",
		pathStep(installer, install.Request{NoPath: true}, install.Outcome{}))
	assert.Equal(t, "recorded "+entry,
		pathStep(installer, install.Request{}, install.Outcome{PathEntry: entry, PathChanged: true}))
	assert.Equal(t, "no longer recorded: "+entry,
		pathStep(remover, install.Request{}, install.Outcome{PathEntry: entry, PathChanged: true}))
	assert.Contains(t, pathStep(installer, install.Request{}, install.Outcome{PathEntry: entry}),
		install.PathStepNothingToDo,
		"why nothing happened is this system's own answer, not a sentence written here")
}

// Something that is not a failure and is not for the log either: a drop-in
// directory that was there and could not be used is why the file the report names
// is the one it is, so it is printed under the report where the person is already
// looking.
func TestSomethingWorthKnowingThatIsNotAFailureIsPrintedWithTheReport(t *testing.T) {
	var out bytes.Buffer

	reportWiring(&out, wiring{name: "install", headline: "wired the sshakku hook into one shell:"},
		install.Request{NoPath: true},
		install.Outcome{
			Shell: "bash", Wired: filepath.Join("home", "startup"),
			Notes: []string{"Profile.d is there but nothing loads it"},
		})

	assert.Contains(t, out.String(), "note: Profile.d is there but nothing loads it")
}

// The hook runs this binary by its own path, so an install that cannot find out
// where it is has nothing to write into the hook — and says so instead of wiring
// something that would run whatever happened to be on PATH at login.
func TestAnInstallThatCannotFindThisProgramSaysSoAndWritesNothing(t *testing.T) {
	installInto(t, t.TempDir())
	profile := aStartupFile(t)
	exe, _ := aWiredShell(t)
	d := realDeps()
	d.self = func() (string, error) { return "", errThisProgramHasNoPath }
	var stdout, stderr bytes.Buffer

	code := d.run(t.Context(), &stdout, &stderr,
		[]string{"install", "--shell-exe", exe, "--profile", profile, "--no-path"})

	assert.Equal(t, 1, code, "a command that ran and could not do the thing, not a usage error")
	assert.Contains(t, stderr.String(), "this program's own path")
	assert.NoFileExists(t, profile)
}
