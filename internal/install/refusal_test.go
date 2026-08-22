package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
)

// spellingThatRefuses points the install at a system whose shell spells paths
// differently from this program and whose translator will not answer.
//
// Which direction refuses is the argument, because the two are met at different
// moments: a path this program cannot open stops the install before anything is
// written, while one it cannot render the shell's way stops it after the hook
// exists. Both have to be handled, and on a system where the two spellings agree
// neither could otherwise be reached.
func spellingThatRefuses(t *testing.T, forShell, forUs bool) {
	t.Helper()

	refuse := func(context.Context, string) (string, error) {
		return "", errors.New("no path translator answered")
	}
	previous := spellingForShell
	spellingForShell = func(string) spelling {
		s := spelling{}
		if forShell {
			s.toShell = refuse
		}
		if forUs {
			s.toUs = refuse
		}
		return s
	}
	t.Cleanup(func() { spellingForShell = previous })
}

// searchListThatRefuses points the install at a system that keeps a stored search
// list and will not have it written. On a system with nowhere to record one there
// is no refusal to be had, and the handling of one would be unreachable.
func searchListThatRefuses(t *testing.T) {
	t.Helper()

	refuse := func(Scope, string) (bool, error) { return false, errors.New("the environment could not be written") }
	previousAdd, previousRemove := addToPath, removeFromPath
	addToPath, removeFromPath = refuse, refuse
	t.Cleanup(func() { addToPath, removeFromPath = previousAdd, previousRemove })
}

// wiringRequest is an install of the file named, for the shell this machine
// really has, with nothing else in play.
func wiringRequest(t *testing.T, home, profile string) Request {
	t.Helper()

	exe, kind := aLiveInterpreter(t)
	return Request{
		Shell: kind, ShellExe: exe, Scope: User, Hosts: AllHosts, Profile: profile,
		Binary: filepath.Join(home, "bin", "sshakku"),
	}
}

// F44: everything that can refuse is asked before anything is written, and a
// refusal says which step met it. Each case below is one of the steps, made to
// fail for the reason a person could really meet it for.
func TestAnInstallThatCannotFinishSaysWhichStepStoppedIt(t *testing.T) {
	t.Run("the interpreter named is not a shell", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		notAShell := filepath.Join(home, "notepad")
		require.NoError(t, os.WriteFile(notAShell, []byte("not a shell"), 0o755))

		_, err := Install(t.Context(), Request{Shell: Auto, ShellExe: notAShell, Scope: User, NoPath: true}, Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), notAShell)
	})

	t.Run("the environment names no directory to install into", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, "")
		profile := filepath.Join(home, "startup-file")

		_, err := Install(t.Context(), wiringRequest(t, home, profile), Ancestry{})

		require.Error(t, err)
		assert.NoFileExists(t, profile, "the wiring is not written when there is nowhere for the hook to go")
	})

	t.Run("the directory for the hook cannot be made", func(t *testing.T) {
		home := t.TempDir()
		inTheWay := filepath.Join(home, "a-file-not-a-directory")
		require.NoError(t, os.WriteFile(inTheWay, []byte("mine"), 0o644))
		installInto(t, inTheWay)
		profile := filepath.Join(home, "startup-file")

		_, err := Install(t.Context(), wiringRequest(t, home, profile), Ancestry{})

		require.Error(t, err)
		assert.NoFileExists(t, profile, "and the startup file is left alone, since the hook it would run is not there")
	})

	t.Run("the path cannot be rendered the way the shell reads one", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		spellingThatRefuses(t, true, false)

		_, err := Install(t.Context(), wiringRequest(t, home, filepath.Join(home, "startup-file")), Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "translator",
			"a hook wired with a path the shell cannot open is one that fails at every login with nothing to say why")
	})

	t.Run("the startup file cannot be written", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		// A directory that is not there is made, so an absent one refuses
		// nothing; one that cannot be made is what stops an install, and a file
		// where the directory would go is that.
		inTheWay := filepath.Join(home, "not-a-directory")
		require.NoError(t, os.WriteFile(inTheWay, []byte("something of somebody's own"), 0o644))
		profile := filepath.Join(inTheWay, "startup-file")

		_, err := Install(t.Context(), wiringRequest(t, home, profile), Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), profile, "the file that could not be wired is the one to name")
	})

	t.Run("the search list will not be written", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		searchListThatRefuses(t)
		req := wiringRequest(t, home, filepath.Join(home, "startup-file"))
		req.NoPath = false

		out, err := Install(t.Context(), req, Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), out.PathEntry, "which directory could not be recorded is what to say")
		assert.NotEmpty(t, out.Wired,
			"and what was already done is still reported: the wiring is there and a person has to know it")
	})
}

// The same for taking it back out. An uninstall reaches further than an install —
// it sweeps every file an install could have chosen — so each of these is a step
// an install never meets.
func TestAnUninstallThatCannotFinishSaysWhichStepStoppedIt(t *testing.T) {
	t.Run("the interpreter named is not a shell", func(t *testing.T) {
		installInto(t, t.TempDir())

		_, err := Uninstall(t.Context(), Request{Shell: Auto, ShellExe: "not-a-program", Scope: User, NoPath: true}, Ancestry{})

		assert.Error(t, err)
	})

	t.Run("something that is not a directory is where a drop-in directory would be", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		profile := filepath.Join(home, "startup-file")
		require.NoError(t, os.WriteFile(profile, []byte("# mine\n"), 0o644))
		// A file where the drop-in directory would be is neither a directory nor
		// nothing: reported as absent, the hook would go into the profile and the
		// real problem would go unmentioned.
		require.NoError(t, os.WriteFile(dropInDirBeside(profile), []byte("not a directory"), 0o644))

		_, err := Uninstall(t.Context(), wiringRequest(t, home, profile), Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("the file the wiring would be in cannot be read", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		// A directory where the startup file should be: whatever that is, it is
		// not a file with our block in it, and rewriting it is not something to
		// attempt quietly.
		profile := filepath.Join(home, "startup-file")
		require.NoError(t, os.Mkdir(profile, 0o755))

		_, err := Uninstall(t.Context(), wiringRequest(t, home, profile), Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), profile)
	})

	t.Run("the environment names no directory the hook could be in", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, "")

		_, err := Uninstall(t.Context(), wiringRequest(t, home, filepath.Join(home, "startup-file")), Ancestry{})

		assert.Error(t, err)
	})

	t.Run("the hook cannot be removed", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		req := wiringRequest(t, home, filepath.Join(home, "startup-file"))

		// Where the rendered hook belongs, with something else inside it. A
		// directory that still holds something will not be removed, which is the
		// difference between this and the hook simply not being there.
		locations, err := LocationsFor(req.Scope)
		require.NoError(t, err)
		// The hook's name is the wired shell's, and this machine's own shell
		// decides which that is: a name written out here would be the other
		// platform's, and the directory would be one the uninstall never looks at.
		hook := filepath.Join(locations.HookDir, plan{kind: req.Shell}.hookName())
		require.NoError(t, os.MkdirAll(hook, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(hook, "something-else"), []byte("mine"), 0o644))

		_, err = Uninstall(t.Context(), req, Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), hook)
	})

	t.Run("the search list will not be written", func(t *testing.T) {
		home := t.TempDir()
		installInto(t, home)
		searchListThatRefuses(t)
		req := wiringRequest(t, home, filepath.Join(home, "startup-file"))
		req.NoPath = false

		_, err := Uninstall(t.Context(), req, Ancestry{})

		assert.Error(t, err)
	})
}

// The step that makes the program runnable by name is taken unless it is
// declined, and what it did is reported rather than assumed — including on a
// system where what it does is nothing.
func TestTheSearchListStepIsTakenUnlessItIsDeclined(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	req := wiringRequest(t, home, filepath.Join(home, "startup-file"))
	req.NoPath = false

	installed, err := Install(t.Context(), req, Ancestry{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "bin"), installed.PathEntry,
		"the directory in question is the one the binary is in, which is what a session would have to search")

	removed, err := Uninstall(t.Context(), req, Ancestry{})
	require.NoError(t, err)
	assert.Equal(t, installed.PathEntry, removed.PathEntry, "and the same one on the way out")

	req.NoPath = true
	declined, err := Install(t.Context(), req, Ancestry{})
	require.NoError(t, err)
	assert.Empty(t, declined.PathEntry, "declined, there is no entry to report at all")
	assert.False(t, declined.PathChanged)
}

// Only something worth reading becomes a note. A note that said nothing would
// print an empty `note:` line under the report, which reads as something having
// gone unsaid.
func TestOnlySomethingWorthSayingBecomesANote(t *testing.T) {
	p := plan{}

	p.note("")
	p.note("   \n")
	assert.Empty(t, p.notes)

	p.note("the drop-in directory was there and could not be used")
	assert.Len(t, p.notes, 1)
}

// A drop-in directory this install has to make, in a place it cannot: reported
// with the directory named, rather than a hook written nowhere and called done.
func TestADropInDirectoryThatCannotBeMadeIsReported(t *testing.T) {
	inTheWay := filepath.Join(t.TempDir(), "a-file-not-a-directory")
	require.NoError(t, os.WriteFile(inTheWay, []byte("mine"), 0o644))
	p := bournePlan(t)
	require.NoError(t, p.forMachine(t.Context(), machineWiring{DropInDir: filepath.Join(inTheWay, "profile.d")}))

	err := p.writeDropIn(". '/hook.sh'")

	require.Error(t, err)
	assert.Contains(t, err.Error(), p.dropInDir)
}

// Naming a kind of shell instead of a program has this system look for one, and
// the answer is judged rather than taken from the name it was found under.
func TestNamingAKindOfShellFindsOneOnThisSystem(t *testing.T) {
	_, kind := aLiveInterpreter(t)

	found, exe, err := whichShell(t.Context(), Request{Shell: kind}, Ancestry{})

	require.NoError(t, err)
	assert.Equal(t, kind, found)
	assert.NotEmpty(t, exe, "and the program found is the one that will be asked about itself")
}

// A kind this system can wire, with no such program anywhere a session would look
// for one: the refusal says what was looked for, because the remedy is to name the
// program and that is easier when you can see which names were tried.
func TestAKindWithNoProgramToBeFoundIsRefusedWithWhatWasTried(t *testing.T) {
	_, kind := aLiveInterpreter(t)
	// Every directory a session would search, and nothing in any of them.
	t.Setenv("PATH", t.TempDir())

	_, _, err := whichShell(t.Context(), Request{Shell: kind}, Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--shell-exe", "the remedy travels with the refusal")
}

// Where the two spellings differ and the translator will not answer, a path this
// program cannot open stops the install before anything is written — including
// while it is looking to see which startup file exists, where an unanswerable
// path is not a file it can say it found.
func TestAPathThatCannotBeTranslatedForUsStopsTheInstallBeforeItWrites(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	spellingThatRefuses(t, false, true)
	exe, kind := aLiveInterpreter(t)

	_, err := Install(t.Context(), Request{
		Shell: kind, ShellExe: exe, Scope: User, Hosts: AllHosts, NoPath: true,
		Binary: filepath.Join(home, "bin", "sshakku"),
	}, Ancestry{})

	require.Error(t, err)
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		assert.NoFileExists(t, filepath.Join(home, name))
	}
}

// An uninstall sweeps every file an install could have chosen, so it can meet a
// refusal about a file this install never wired — and it stops there rather than
// carrying on and reporting a clean sweep it did not make.
func TestAnUninstallStopsOnAFileItCannotJudge(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	exe, kind := aLiveInterpreter(t)
	if kind != Bash && kind != Zsh {
		t.Skip("this system's own interpreter wires a profile rather than a sweep of login files")
	}
	// The file the shell will read, and something that is not a directory where
	// another candidate's drop-in directory would be. The install's own choice is
	// sound; the sweep meets the other one.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".profile"), []byte("# mine\n"), 0o644))
	require.NoError(t, os.WriteFile(dropInDirBeside(filepath.Join(home, ".bash_profile")), []byte("x"), 0o644))

	_, err := Uninstall(t.Context(), Request{
		Shell: kind, ShellExe: exe, Scope: User, Hosts: AllHosts, NoPath: true,
		Binary: filepath.Join(home, "bin", "sshakku"),
	}, Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

// An interpreter that answers to a shell's name and cannot be asked about itself
// is reported as that, rather than wired on the strength of the name.
func TestAShellThatCannotBeAskedAboutItselfIsReported(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	_, kind := aLiveInterpreter(t)
	if kind != Bash && kind != Zsh {
		t.Skip("this system's own interpreter is asked with a different query")
	}
	// A file under the name this system's table knows, which is not a program.
	name, _ := aShellName()
	notReallyAShell := filepath.Join(home, filepath.Base(name))
	require.NoError(t, os.WriteFile(notReallyAShell, []byte("not a program at all"), 0o644))

	_, err := Install(t.Context(), Request{
		Shell: Auto, ShellExe: notReallyAShell, Scope: User, Hosts: AllHosts, NoPath: true,
		Binary: filepath.Join(home, "bin", "sshakku"),
	}, Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), notReallyAShell, "which program could not answer is the whole of the report")
}

// A machine-wide install for a shell this system has no answer for stops at the
// table, before anything is chosen or written.
func TestAMachineWideInstallForAShellWithNoAnswerStopsAtTheTable(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)

	var refused bool
	for _, kind := range offeredShellKinds() {
		if _, err := machineWiringFor(kind); err == nil {
			continue
		}
		if _, ok := RecogniseShell(string(kind)); !ok {
			// A kind this system's table names but does not recognise under that
			// bare name: what it is called as a file is that platform's business,
			// and this test is about the machine-wide table rather than about
			// recognising an interpreter.
			continue
		}
		refused = true
		// A file under that shell's name: what is being exercised is the table,
		// which is read before the interpreter is asked anything.
		exe := filepath.Join(home, string(kind))
		require.NoError(t, os.WriteFile(exe, []byte("a shell"), 0o755))

		_, err := resolve(t.Context(), Request{Shell: Auto, ShellExe: exe, Scope: Machine}, Ancestry{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--profile")
	}
	if !refused {
		t.Skip("this system answers for every Bourne shell it can wire machine-wide")
	}
}

// A PowerShell profile that cannot be judged stops the install: where the hook
// goes depends on that file, and the two answers are different files.
func TestAProfileThatCannotBeJudgedStopsAPowerShellInstall(t *testing.T) {
	spellingThatRefuses(t, false, true)
	p := plan{kind: PowerShellCore, spelling: spellingForShell("anything")}
	p.dialect = dialect(t, shell.PowerShell)
	host, err := parseHost([]byte(powerShell7Answer))
	require.NoError(t, err)

	err = p.forHost(t.Context(), host, Request{Scope: User, Hosts: AllHosts})

	require.Error(t, err)
	assert.Empty(t, p.placement.Path)
}
