package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
)

// bournePlan is a plan for a Bourne shell whose paths are spelt the way this
// program spells them, which is every plan on a system with no translator and
// the shape of the rest on the one that has one.
func bournePlan(t *testing.T) plan {
	t.Helper()
	dialect, err := shell.Named(shell.Posix)
	require.NoError(t, err)
	return plan{kind: Bash, dialect: dialect}
}

func powerShellPlan(t *testing.T) plan {
	t.Helper()
	dialect, err := shell.Named(shell.PowerShell)
	require.NoError(t, err)
	return plan{kind: WindowsPowerShell, dialect: dialect}
}

func TestTheRenderedHookIsWrittenWhereTheWiringWillPointAtIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "there", "yet")

	path, err := renderInto(dir, bournePlan(t), "/opt/sshakku/bin/sshakku")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "shell-hook.sh"), path,
		"the directory is made on the way, since a first install finds none")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assertRenderedFrom(t, content, bourneHookTemplate, BourneBinaryPlaceholder,
		shell.Quote("/opt/sshakku/bin/sshakku"))
	assert.NotContains(t, string(content), BourneBinaryPlaceholder,
		"the placeholder is a plausible path, so one left behind would be run rather than noticed")
}

// assertRenderedFrom says which of the two templates a rendered hook was made
// from, by putting the placeholder back where the path went and comparing what
// is left with the file itself.
//
// Saying only that the path arrived would not do it: both templates hold a
// single-quoted literal, so a hook rendered from the wrong one carries the path
// just as convincingly — and is a file the shell reports as its own syntax
// error, at every login.
func assertRenderedFrom(t *testing.T, rendered, template []byte, placeholder, quoted string) {
	t.Helper()
	assert.Equal(t, string(template), strings.ReplaceAll(string(rendered), quoted, placeholder))
}

// Windows PowerShell reads a script file with no byte-order mark in the
// machine's ANSI code page. A hook whose path holds a character outside that
// page then names a file that is not there, and the failure appears at every
// login of an account whose name is not ASCII.
func TestTheRenderedPowerShellHookKeepsItsByteOrderMark(t *testing.T) {
	dir := t.TempDir()

	path, err := renderInto(dir, powerShellPlan(t), `C:\Users\Ástríður\sshakku.exe`)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "shell-hook.ps1"), path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(content), "\ufeff"), "the mark travels with the template")
	assert.Contains(t, string(content), "Ástríður", "and the path arrives whole")
	assertRenderedFrom(t, content, powerShellHookTemplate, PowerShellBinaryPlaceholder,
		dialect(t, shell.PowerShell).Quote(`C:\Users\Ástríður\sshakku.exe`))
}

func TestTheWiringRunsTheHookInTheLanguageOfTheShellItIsFor(t *testing.T) {
	for _, c := range []struct {
		name string
		p    plan
		hook string
		want string
	}{
		{
			"bourne", bournePlan(t), "/home/o'brien/.local/share/sshakku/shell-hook.sh",
			`. '/home/o'\''brien/.local/share/sshakku/shell-hook.sh'`,
		},
		{
			"powershell", powerShellPlan(t), `C:\Users\O'Brien\sshakku\shell-hook.ps1`,
			`. 'C:\Users\O''Brien\sshakku\shell-hook.ps1'`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			line, err := c.p.sourceLine(t.Context(), c.hook)

			require.NoError(t, err)
			assert.Equal(t, c.want, line, "an apostrophe in the path does not end the literal")
		})
	}
}

// The two kinds of target differ in the language of everything they touch, and
// the names are what a person finds on disk afterwards. A drop-in of one
// language in the other's directory is a file the shell reads and reports as
// its own syntax error, at every login.
func TestEachKindOfTargetGetsItsOwnLanguageThroughout(t *testing.T) {
	bourne, powerShell := bournePlan(t), powerShellPlan(t)

	assert.Equal(t, "shell-hook.sh", bourne.hookName())
	assert.Equal(t, "50-sshakku-init.sh", bourne.dropInName())
	assert.True(t, strings.HasPrefix(string(bourne.dropInFile(". '/hook.sh'")), "#!/bin/bash\n"),
		"some of the shells that read a drop-in directory run each file in it rather than sourcing it")

	assert.Equal(t, "shell-hook.ps1", powerShell.hookName())
	assert.Equal(t, "50-sshakku-init.ps1", powerShell.dropInName())
	assert.NotContains(t, string(powerShell.dropInFile(". 'C:\\hook.ps1'")), "#!/bin/bash",
		"PowerShell has no use for an interpreter line and would only have it to read")

	assert.Equal(t, shell.Posix, dialectName(Bash))
	assert.Equal(t, shell.Posix, dialectName(Zsh))
	assert.Equal(t, shell.PowerShell, dialectName(PowerShellCore))
	assert.Equal(t, shell.PowerShell, dialectName(WindowsPowerShell))
}

func TestAProgramNamedOutrightIsTheOneUsed(t *testing.T) {
	name, kind := aShellName()

	got, exe, err := whichShell(t.Context(), Request{ShellExe: name, Shell: Auto}, Ancestry{})

	require.NoError(t, err)
	assert.Equal(t, kind, got)
	assert.Equal(t, name, exe, "it is used as given, not looked up again")
}

// Naming a program and a kind that disagree is a mistake with two plausible
// readings, and acting on either silently would wire something the person did
// not ask for.
func TestAProgramAndAKindThatDisagreeAreRefused(t *testing.T) {
	name, kind := aShellName()
	other := Bash
	if kind == Bash {
		other = PowerShellCore
	}

	_, _, err := whichShell(t.Context(), Request{ShellExe: name, Shell: other}, Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), name)
	assert.Contains(t, err.Error(), string(other))
}

func TestAProgramThisSystemDoesNotKnowIsRefused(t *testing.T) {
	_, _, err := whichShell(t.Context(), Request{ShellExe: filepath.Join("somewhere", "vi")}, Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "vi")
}

func TestAKindThisSystemHasIsFoundWithoutBeingNamed(t *testing.T) {
	want, kind := aLiveInterpreter(t)

	exe, err := lookInterpreter(t.Context(), kind)

	require.NoError(t, err)
	assert.Equal(t, want, exe)
}

// The refusal has to name what was looked at: the remedy is --shell-exe, and
// somebody can only supply the missing path if they can see which ones were
// already tried.
func TestAKindThisSystemHasNoInterpreterForIsRefusedByName(t *testing.T) {
	_, err := lookInterpreter(t.Context(), ShellKind("ksh"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ksh")
}

// With nothing named at all, the shell is the one that ran the command: the
// ancestry of this very process is read until something in it is a shell this
// system can wire. That is what makes `sshakku install`, typed in a window,
// wire that window's shell.
func TestWithNothingNamedTheShellIsTheOneThatRanTheCommand(t *testing.T) {
	name, kind := aShellName()
	tree := Ancestry{
		PID: 100,
		Source: fakeAncestry{
			100: {ppid: 200, name: "sshakku.exe"},
			200: {ppid: 0, name: filepath.Base(name)},
		},
		Image: func(pid int) (string, bool) { return name, pid == 200 },
	}

	got, exe, err := whichShell(t.Context(), Request{Shell: Auto}, tree)

	require.NoError(t, err)
	assert.Equal(t, kind, got)
	assert.Equal(t, name, exe, "the whole path, which is what tells two programs of one name apart")
}

// Nothing named and nothing said: an empty request is the same question as
// --shell=auto, since the zero value of a flag nobody set has to mean what the
// default means.
func TestAnEmptyRequestAsksTheAncestryToo(t *testing.T) {
	name, kind := aShellName()
	tree := Ancestry{
		PID:    100,
		Source: fakeAncestry{100: {ppid: 0, name: filepath.Base(name)}},
		Image:  func(int) (string, bool) { return name, true },
	}

	got, _, err := whichShell(t.Context(), Request{}, tree)

	require.NoError(t, err)
	assert.Equal(t, kind, got)
}

// A profile that will not run is not a wiring that half works — it is one that
// would never have run at all, so it is reported instead of written.
func TestAPowerShellThatWillNotRunItsProfileIsReportedWithTheRemedy(t *testing.T) {
	for _, c := range []struct{ name, policy, mode, wants string }{
		{"restricted", "Restricted", "FullLanguage", "Set-ExecutionPolicy"},
		{"all signed", "AllSigned", "FullLanguage", "Set-ExecutionPolicy"},
		{"constrained", "RemoteSigned", "ConstrainedLanguage", "ConstrainedLanguage"},
	} {
		t.Run(c.name, func(t *testing.T) {
			note, willNot := willNotRunItsProfile(Host{
				EffectiveExecutionPolicy: c.policy,
				LanguageMode:             c.mode,
			})

			assert.True(t, willNot)
			assert.Contains(t, note, c.wants)
		})
	}
}

func TestAnOrdinaryPowerShellIsWiredWithoutComment(t *testing.T) {
	note, willNot := willNotRunItsProfile(Host{
		EffectiveExecutionPolicy: "RemoteSigned",
		LanguageMode:             "FullLanguage",
	})

	assert.False(t, willNot)
	assert.Empty(t, note)
}

// A host that said nothing about its language mode is not a host in a
// restricted one. Refusing on an empty answer would refuse every interpreter
// whose query lost that field.
func TestAHostThatNamedNoLanguageModeIsNotRefusedForIt(t *testing.T) {
	_, willNot := willNotRunItsProfile(Host{EffectiveExecutionPolicy: "RemoteSigned"})

	assert.False(t, willNot)
}

// The round trip, driven through the real interpreter this machine has: the
// wiring goes into the file named, points at a hook that names the binary, and
// an uninstall gives the file back exactly as it was found.
func TestWiringAFileAndUnwiringItLeavesItAsItWasFound(t *testing.T) {
	exe, kind := aLiveInterpreter(t)
	home := t.TempDir()
	installInto(t, home)

	startup := filepath.Join(home, "startup-file")
	before := "# something the user wrote\nexport EDITOR=vi\n"
	require.NoError(t, os.WriteFile(startup, []byte(before), 0o644))

	binary := filepath.Join(home, "bin", "sshakku")
	req := Request{Shell: kind, ShellExe: exe, Scope: User, Hosts: AllHosts, Profile: startup, NoPath: true, Binary: binary}

	installed, err := Install(t.Context(), req, Ancestry{})
	require.NoError(t, err)

	assert.Equal(t, kind, installed.Shell)
	assert.Equal(t, startup, installed.Wired)
	assert.False(t, installed.DropIn, "there is no drop-in directory beside this file")
	assert.Empty(t, installed.PathEntry, "--no-path was asked for")

	wired, err := os.ReadFile(startup)
	require.NoError(t, err)
	assert.Contains(t, string(wired), before, "what the user wrote is still there")
	assert.Contains(t, string(wired), MarkerStart)
	assert.Contains(t, string(wired), filepath.Base(installed.HookFile),
		"the block runs the hook that was rendered")

	hook, err := os.ReadFile(installed.HookFile)
	require.NoError(t, err)
	assert.Contains(t, string(hook), filepath.Base(binary))

	removed, err := Uninstall(t.Context(), req, Ancestry{})
	require.NoError(t, err)

	assert.Equal(t, startup, removed.Wired)
	assert.Equal(t, installed.HookFile, removed.HookFile)
	assert.NoFileExists(t, installed.HookFile, "the hook goes with the wiring that pointed at it")
	after, err := os.ReadFile(startup)
	require.NoError(t, err)
	assert.Equal(t, before, string(after), "byte for byte what was there before")
}

// Installing twice is what a person does after upgrading, and it must leave one
// block rather than two — nothing complains about the second, and both would
// run at every login.
func TestInstallingTwiceLeavesOneWiring(t *testing.T) {
	exe, kind := aLiveInterpreter(t)
	home := t.TempDir()
	installInto(t, home)

	startup := filepath.Join(home, "startup-file")
	req := Request{
		Shell: kind, ShellExe: exe, Scope: User, Hosts: AllHosts, Profile: startup, NoPath: true,
		Binary: filepath.Join(home, "bin", "sshakku"),
	}

	_, err := Install(t.Context(), req, Ancestry{})
	require.NoError(t, err)
	once, err := os.ReadFile(startup)
	require.NoError(t, err)

	_, err = Install(t.Context(), req, Ancestry{})
	require.NoError(t, err)
	twice, err := os.ReadFile(startup)
	require.NoError(t, err)

	assert.Equal(t, string(once), string(twice))
	assert.Equal(t, 1, strings.Count(string(twice), MarkerStart))
}

// With no file named, the shell itself is asked where it looks — and it is
// asked rather than assembled, because on a POSIX-emulating environment its
// home is not this program's idea of one, and neither is the spelling of the
// path it will read.
//
// The file chosen is the one that shell will really read: a login shell reads
// the first of the three it finds and no others, so wiring .bash_profile on an
// account set up with .profile does not add SSHakku to that account's
// configuration — it replaces it.
func TestWithNoFileNamedTheShellIsAskedWhereItLooks(t *testing.T) {
	for _, c := range []struct{ name, existing, want string }{
		{"a home with nothing in it", "", ".bash_profile"},
		{"a home set up with .profile", ".profile", ".profile"},
		{"a home set up with .bash_login", ".bash_login", ".bash_login"},
	} {
		t.Run(c.name, func(t *testing.T) {
			exe, err := lookInterpreter(t.Context(), Bash)
			require.NoError(t, err, "this suite needs a Bourne shell to ask")

			home := t.TempDir()
			installInto(t, home)
			t.Setenv("HOME", home)
			if c.existing != "" {
				require.NoError(t, os.WriteFile(filepath.Join(home, c.existing), []byte("# mine\n"), 0o644))
			}

			req := Request{
				Shell: Bash, ShellExe: exe, Scope: User, NoPath: true,
				Binary: filepath.Join(home, "bin", "sshakku"),
			}

			installed, err := Install(t.Context(), req, Ancestry{})
			require.NoError(t, err)

			assert.Equal(t, c.want, filepath.Base(installed.Wired))
			wired, err := os.ReadFile(installed.Wired)
			require.NoError(t, err)
			assert.Contains(t, string(wired), MarkerStart)
			assert.NotContains(t, string(wired), `\`,
				"the line is read by the shell, so the path in it is in the shell's own spelling")
			for _, other := range loginFiles {
				if other != c.want && other != c.existing {
					assert.NoFileExists(t, filepath.Join(home, other),
						"creating this one would stop the shell reading the one that is wired")
				}
			}

			_, err = Uninstall(t.Context(), req, Ancestry{})
			require.NoError(t, err)

			after, err := os.ReadFile(installed.Wired)
			if c.existing == "" {
				assert.True(t, os.IsNotExist(err),
					"a file created for the wiring goes when the wiring goes: an account that had no"+
						" login file before has none after")
			} else {
				require.NoError(t, err)
				assert.Equal(t, "# mine\n", string(after))
			}
			assert.NoFileExists(t, installed.HookFile)
		})
	}
}

// F19, F44: a machine-wide install wires what every account of the machine
// reads. The account that ran the command is not that, and getting this wrong
// is invisible from the inside: the hook lands in a directory the whole machine
// shares, so an administrator who installed for everybody has installed for
// themselves and has every reason to believe otherwise.
//
// Only the plan is asked for, and deliberately: what a machine-wide target
// names is outside any directory a test owns, so resolving it is the whole of
// what can be checked without writing into the system running the suite.
func TestAMachineWideInstallWiresTheMachineAndNotTheAccountThatRanIt(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	t.Setenv("HOME", home)
	exe, kind := aLiveInterpreter(t)

	p, err := resolve(t.Context(), Request{Shell: kind, ShellExe: exe, Scope: Machine, Hosts: AllHosts}, Ancestry{})

	require.NoError(t, err)
	require.NotEmpty(t, p.placement.Path)
	assert.NotContains(t, p.placement.Path, home,
		"a machine-wide wiring in one account's own startup file reaches one person")
	for _, place := range p.sweep {
		assert.NotContains(t, place.Path, home,
			"and an uninstall that swept that account's files would be answering the same wrong question")
	}
}

// The default PowerShell target is the profile every host of that interpreter
// reads, of the scope asked for — and the uninstall sweep is every profile the
// interpreter named, because somebody may have installed with one set of flags
// and be uninstalling with another.
func TestThePowerShellTargetIsTheOneTheInterpreterNamed(t *testing.T) {
	dir := t.TempDir()
	host := Host{
		EffectiveExecutionPolicy: "RemoteSigned",
		LanguageMode:             "FullLanguage",
		Profiles: Profiles{
			Default:                filepath.Join(dir, "Microsoft.PowerShell_profile.ps1"),
			CurrentUserAllHosts:    filepath.Join(dir, "profile.ps1"),
			CurrentUserCurrentHost: filepath.Join(dir, "Microsoft.PowerShell_profile.ps1"),
			AllUsersAllHosts:       filepath.Join(dir, "all", "profile.ps1"),
			AllUsersCurrentHost:    filepath.Join(dir, "all", "Microsoft.PowerShell_profile.ps1"),
		},
	}

	for _, c := range []struct {
		name  string
		req   Request
		want  string
		sweep int
	}{
		{"the default", Request{Scope: User, Hosts: AllHosts}, host.Profiles.CurrentUserAllHosts, 4},
		{"one host", Request{Scope: User, Hosts: CurrentHost}, host.Profiles.CurrentUserCurrentHost, 4},
		{"every account", Request{Scope: Machine, Hosts: AllHosts}, host.Profiles.AllUsersAllHosts, 4},
		{
			"a file named outright",
			Request{Scope: User, Hosts: AllHosts, Profile: filepath.Join(dir, "mine.ps1")},
			filepath.Join(dir, "mine.ps1"), 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := powerShellPlan(t)

			require.NoError(t, p.forHost(t.Context(), host, c.req))

			assert.Equal(t, c.want, p.placement.Path)
			assert.Len(t, p.sweep, c.sweep,
				"five names are four files, and somebody who named a file named the whole of what to touch")
		})
	}
}

// The same host, refused: the check happens before a target is even chosen, so
// nothing is written and nothing is reported as wired.
func TestAHostThatWillNotRunItsProfileIsRefusedBeforeATargetIsChosen(t *testing.T) {
	p := powerShellPlan(t)

	err := p.forHost(t.Context(), Host{
		EffectiveExecutionPolicy: "Restricted",
		Profiles:                 Profiles{CurrentUserAllHosts: filepath.Join(t.TempDir(), "profile.ps1")},
	}, Request{Scope: User, Hosts: AllHosts})

	require.Error(t, err)
	assert.Empty(t, p.placement.Path)
	assert.Empty(t, p.sweep)
}

// An uninstall runs on machines that were never installed — somebody making
// sure, or a script that removes before it installs.
func TestUninstallingWhatWasNeverInstalledIsNotAFailure(t *testing.T) {
	exe, kind := aLiveInterpreter(t)
	home := t.TempDir()
	installInto(t, home)

	startup := filepath.Join(home, "startup-file")
	req := Request{
		Shell: kind, ShellExe: exe, Scope: User, Hosts: AllHosts, Profile: startup, NoPath: true,
		Binary: filepath.Join(home, "bin", "sshakku"),
	}

	_, err := Uninstall(t.Context(), req, Ancestry{})

	require.NoError(t, err)
	assert.NoFileExists(t, startup, "nothing was wired here, so nothing is created to unwire")
}
