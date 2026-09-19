package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/cli/crossuser"
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
	"github.com/OrbintSoft/sshakku/internal/sshtools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errKeyringBoom = errors.New("keyring boom")
	errMustNotRun  = errors.New("must not run")
)

// fakeTokenSource stands in for crossuser.Exec so doctorCrossUser runs without
// root or a second uid: it returns a canned token, or an error to drive the
// read-failure branch.
type fakeTokenSource struct {
	token string
	err   error
}

func (f fakeTokenSource) ReadToken(context.Context, int, int) (string, error) { return f.token, f.err }

var _ crossuser.Source = fakeTokenSource{}

// doctorDeps builds a deps whose report gathering, cross-user token read, and
// effective uid are all faked, so doctor's branches run against synthetic state
// instead of this host's live agent, kernel keyring, and privilege level. gather
// returns report; tokenSource yields a fixed token; geteuid reports euid.
func doctorDeps(report diagnose.Report, ts crossuser.Source, euid int) deps {
	d := depsWithEnsurer(fakeEnsurer{})
	d.gather = func(context.Context, paths.Env, paths.Layout, config.Settings) diagnose.Report { return report }
	d.tokenSource = ts
	d.geteuid = func() int { return euid }
	return d
}

// TestDoctorArgErrors covers doctor's argument-parsing rejections: each returns
// exit code 2 with a diagnostic, without touching the agent or the report.
func TestDoctorArgErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown argument", []string{"--nope"}, "unknown argument"},
		{"--user without a value", []string{"--user"}, "--user requires a value"},
		{"invalid --test-backend name", []string{"--test-backend", "bogus"}, "is not a wallet this system has"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
			var out, errOut bytes.Buffer
			assert.Equalf(t, 2, d.doctor(t.Context(), &out, &errOut, tc.args), "%v is a usage error", tc.args)
			assert.Contains(t, errOut.String(), tc.want,
				"a user who typed something is owed an answer about what they typed")
		})
	}
}

// TestDoctorUnknownUser covers the resolveTargetUser failure path: an
// unresolvable --user value surfaces as exit code 2.
func TestDoctorUnknownUser(t *testing.T) {
	d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, d.doctor(t.Context(), &out, &errOut, []string{"--user", "no-such-user-xyzzy"}),
		"a user nobody can resolve is a usage error, not a diagnosis of somebody")
	assert.Contains(t, errOut.String(), "--user", "and the answer must name the flag that was wrong")
}

// fabricatedTargetUID is the uid the tests below have the account database hand
// back. It must not be the caller's own, or naming it would not be going
// cross-user at all and the branch under test would never be entered; the tests
// assert that rather than assuming it.
const fabricatedTargetUID = 4242

// crossUserTarget makes the account database name an account, and says this
// build diagnoses another account's session (F61), so what follows is the
// dispatch and the report body rather than whether this machine keeps a
// `nobody` and this platform has uids. Both are upstream of what these tests
// judge, and together they let a body that is platform-neutral run on every
// system that compiles it.
func crossUserTarget(t *testing.T) {
	t.Helper()
	require.NotEqual(t, fabricatedTargetUID, paths.FromOS().UID,
		"the fabricated target must be somebody other than the caller, or nothing cross-user happens")

	origID, origName := userLookupID, userLookup
	t.Cleanup(func() { userLookupID, userLookup = origID, origName })
	home := t.TempDir()
	named := func(name string) (*user.User, error) {
		return &user.User{
			Uid: strconv.Itoa(fabricatedTargetUID), Gid: strconv.Itoa(fabricatedTargetUID),
			Username: name, HomeDir: home,
		}, nil
	}
	userLookup, userLookupID = named, named
}

// crossUserDeps is doctorDeps with the cross-user answer of a system that has
// one, so the dispatch below is reached from either kind of machine.
func crossUserDeps(ts crossuser.Source, euid int) deps {
	d := doctorDeps(diagnose.Report{}, ts, euid)
	d.crossUser = crossUserWorks
	return d
}

// TestDoctorCrossUserRefused covers crossUserGuard's non-root refusal: naming a
// different user while not root is rejected with exit code 2, and never reaches
// the token source.
func TestDoctorCrossUserRefused(t *testing.T) {
	crossUserTarget(t)

	d := crossUserDeps(fakeTokenSource{err: errMustNotRun}, 1000)
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, d.doctor(t.Context(), &out, &errOut, []string{"--user", "somebody-else"}),
		"reading another user's session takes root, and the refusal comes before anything is read")
	assert.Contains(t, errOut.String(), "root", "and must say what would be needed")
}

// TestDoctorReadOnly covers the plain read-only path: doctor formats the gathered
// report and, with no --fix or --test-backend, exits 0 without driving the agent.
func TestDoctorReadOnly(t *testing.T) {
	report := diagnose.Report{FixedSock: "/run/sshakku/agent.sock", EnvSock: "/run/sshakku/agent.sock"}
	d := doctorDeps(report, fakeTokenSource{}, 1000)
	var out, errOut bytes.Buffer
	require.Zerof(t, d.doctor(t.Context(), &out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())
	assert.Contains(t, out.String(), "/run/sshakku/agent.sock", "and it must print what was gathered")
}

// TestDoctorTestBackend covers the --test-backend branch: it runs the secret
// backend probe against an injected in-memory backend and reports PASS. The
// backend comes from depsReturning; the doctor-specific seams are layered on top
// so the report and euid stay synthetic too.
func TestDoctorTestBackend(t *testing.T) {
	tempRuntimeEnv(t)
	d := depsReturning(newMemoryBackend())
	d.gather = func(context.Context, paths.Env, paths.Layout, config.Settings) diagnose.Report {
		return diagnose.Report{}
	}
	d.tokenSource = fakeTokenSource{}
	d.geteuid = func() int { return 1000 }
	var out, errOut bytes.Buffer
	require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--test-backend"}),
		"a wallet that works must be reported as working; stderr=%q", errOut.String())
	assert.Contains(t, out.String(), "backend test: PASS", "and the verdict must be plain")
}

// TestDoctorFix covers the --fix self-heal path against a fake ensurer and a
// synthetic report. A healthy ensure with the shell already wired to the live
// socket exits 0 and prints no re-export hint; a report whose EnvSock differs
// prints the export hint.
func TestDoctorFix(t *testing.T) {
	t.Run("wired shell needs no re-export hint", func(t *testing.T) {
		tempRuntimeEnv(t)
		layout := paths.Resolve(paths.FromOS(), paths.ProbeDir)
		report := diagnose.Report{EnvSock: layout.AgentSock}
		d := doctorDeps(report, fakeTokenSource{}, 1000)
		d.ensurer = fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint(layout.AgentSock)}}
		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())
		assert.NotContains(t, out.String(), "export SSH_AUTH_SOCK",
			"a shell already pointed at the live socket has nothing to re-export")
	})

	t.Run("unwired shell prints the re-export hint", func(t *testing.T) {
		tempRuntimeEnv(t)
		report := diagnose.Report{EnvSock: "/somewhere/else.sock"}
		d := doctorDeps(report, fakeTokenSource{}, 1000)
		d.ensurer = fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}}
		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())
		assert.Contains(t, out.String(), "export SSH_AUTH_SOCK=",
			"a shell pointed somewhere else must be told how to reach the agent that is there")
	})

	t.Run("uncreatable layout returns 1", func(t *testing.T) {
		home := tempRuntimeEnv(t)
		require.NoError(t, os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir"), 0o600),
			"seed a file where the config directory should be")
		d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}),
			"a repair that could not lay out its own directories has not repaired anything")
	})

	t.Run("ensure failure propagates its code", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
		d.ensurer = fakeEnsurer{err: errBoom}
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}),
			"an agent that could not be ensured must not be reported as repaired")
	})
}

// TestDoctorCrossUser covers the root-only cross-user dispatch and body against
// fakes: euid 0 lets crossUserGuard pass, and the fake token source stands in
// for reading the target's keyring. A token read error returns 1; a successful
// read reports on the target's session and returns 0.
func TestDoctorCrossUser(t *testing.T) {
	crossUserTarget(t)

	t.Run("token read failure returns 1", func(t *testing.T) {
		d := crossUserDeps(fakeTokenSource{err: errKeyringBoom}, 0)
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.doctor(t.Context(), &out, &errOut, []string{"--user", "somebody-else"}),
			"a session that could not be reached must not be reported on as though it had been")
		assert.Contains(t, errOut.String(), "keyring boom", "and the reason must reach the caller")
	})

	t.Run("successful read reports the target session", func(t *testing.T) {
		d := crossUserDeps(fakeTokenSource{token: "tok"}, 0)
		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--user", "somebody-else"}),
			"root may look at another user's session; stderr=%q", errOut.String())
		assert.Contains(t, out.String(), "diagnosing uid",
			"and the report must say whose session it is about")
	})
}

// TestGatherReport covers gatherReport directly: with HOME and the XDG dirs at
// fresh temp dirs and no ~/.ssh, it reads the real procfs, sockets, and (empty)
// key dir and returns a report naming the fixed socket, without touching the
// real environment.
func TestGatherReport(t *testing.T) {
	tempRuntimeEnv(t)
	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir)
	report := gatherReport(t.Context(), env, layout, config.Settings{})
	assert.Equal(t, platformEndpoint(layout).Native(), report.FixedSock,
		"the report must be about the endpoint sessions on this system are pointed at")
}

// errSSHWillNotStart is what a build that cannot be run refuses with, standing
// for the machine states this test cannot arrange: an ssh that is not
// executable, a binary for another architecture, a program killed before it
// printed.
var errSSHWillNotStart = errors.New("ssh will not start")

// TestSSHVersionHere covers what the report is told about the OpenSSH a session
// would run for `ssh`: the release a build prints, and the zero value that
// stands for a build which did not say. The two must not collapse into one
// another — a version nobody read is not an old version, and a reader told to
// replace an OpenSSH that was never the problem goes and changes the one thing
// that was working.
//
// Which system the question is asked of is handed in, so a machine with one
// OpenSSH and a machine that has to choose between several are both askable
// from either.
func TestSSHVersionHere(t *testing.T) {
	t.Run("a build that answers is read", func(t *testing.T) {
		r := runtest.NewRunner().On(sshtools.SSHName,
			runtest.Stdout("OpenSSH_9.6p1, OpenSSL 3.0.13 30 Jan 2024\n", 0))
		v := sshVersionHere(t.Context(), r, sshtools.System{})
		assert.Equal(t, 9, v.Major, "the release a build prints is the release reported")
		assert.Equal(t, 6, v.Minor, "and so is the minor, which is what decides whether it can be sent to a helper")
		assert.False(t, v.Unread(), "a build that answered is not one that was never asked")
	})

	t.Run("a build that will not run says nothing about a version", func(t *testing.T) {
		r := runtest.NewRunner().On(sshtools.SSHName, runtest.Fails(errSSHWillNotStart))
		v := sshVersionHere(t.Context(), r, sshtools.System{})
		assert.True(t, v.Unread(), "a question that could not be asked has no answer to report")
		assert.Empty(t, v.Text, "and nothing to quote either")
	})

	t.Run("a system with no ssh it can name is asked nothing", func(t *testing.T) {
		// A system that emulates another keeps more than one build of OpenSSH
		// and they are not interchangeable, so which to run has to be worked
		// out — and with nothing on the session's PATH and nowhere of its own
		// to look, there is no build to name. The empty directory is what makes
		// the lookup fail for the real reason rather than for the host's.
		t.Setenv("PATH", t.TempDir())
		r := runtest.NewRunner()
		v := sshVersionHere(t.Context(), r, sshtools.System{EmulationRuntimes: []string{"msys-2.0.dll"}})
		assert.True(t, v.Unread(), "a build that could not be named was never asked what it is")
		assert.Empty(t, r.Calls, "and nothing may be run at all: there was no program to run")
	})
}
