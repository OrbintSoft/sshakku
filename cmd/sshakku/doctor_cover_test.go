package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// fakeTokenSource stands in for execTokenSource so doctorCrossUser runs without
// root or a second uid: it returns a canned token, or an error to drive the
// read-failure branch.
type fakeTokenSource struct {
	token string
	err   error
}

func (f fakeTokenSource) ReadToken(int, int) (string, error) { return f.token, f.err }

var _ TargetTokenSource = fakeTokenSource{}

// doctorDeps builds a deps whose report gathering, cross-user token read, and
// effective uid are all faked, so doctor's branches run against synthetic state
// instead of this host's live agent, kernel keyring, and privilege level. gather
// returns report; tokenSource yields a fixed token; geteuid reports euid.
func doctorDeps(report diagnose.Report, ts TargetTokenSource, euid int) deps {
	d := depsWithEnsurer(fakeEnsurer{})
	d.gather = func(paths.Env, paths.Layout) diagnose.Report { return report }
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
		{"invalid --test-backend name", []string{"--test-backend", "bogus"}, "unknown backend"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
			var out, errOut bytes.Buffer
			if got := d.doctor(&out, &errOut, tc.args); got != 2 {
				t.Fatalf("doctor(%v) = %d, want 2", tc.args, got)
			}
			if !strings.Contains(errOut.String(), tc.want) {
				t.Errorf("stderr = %q, want it to mention %q", errOut.String(), tc.want)
			}
		})
	}
}

// TestDoctorUnknownUser covers the resolveTargetUser failure path: an
// unresolvable --user value surfaces as exit code 2.
func TestDoctorUnknownUser(t *testing.T) {
	d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
	var out, errOut bytes.Buffer
	if got := d.doctor(&out, &errOut, []string{"--user", "no-such-user-xyzzy"}); got != 2 {
		t.Fatalf("doctor(--user bogus) = %d, want 2", got)
	}
	if !strings.Contains(errOut.String(), "--user") {
		t.Errorf("stderr = %q, want it to name the --user flag", errOut.String())
	}
}

// TestDoctorCrossUserRefused covers crossUserGuard's non-root refusal: naming a
// different real user (nobody) while not root is rejected with exit code 2, and
// never reaches the token source.
func TestDoctorCrossUserRefused(t *testing.T) {
	if _, err := lookupUser("nobody"); err != nil {
		t.Skip("no 'nobody' user on this host")
	}
	d := doctorDeps(diagnose.Report{}, fakeTokenSource{err: errors.New("must not run")}, 1000)
	var out, errOut bytes.Buffer
	if got := d.doctor(&out, &errOut, []string{"--user", "nobody"}); got != 2 {
		t.Fatalf("doctor(--user nobody, non-root) = %d, want 2", got)
	}
	if !strings.Contains(errOut.String(), "root") {
		t.Errorf("stderr = %q, want it to mention the root requirement", errOut.String())
	}
}

// TestDoctorReadOnly covers the plain read-only path: doctor formats the gathered
// report and, with no --fix or --test-backend, exits 0 without driving the agent.
func TestDoctorReadOnly(t *testing.T) {
	report := diagnose.Report{FixedSock: "/run/sshakku/agent.sock", EnvSock: "/run/sshakku/agent.sock"}
	d := doctorDeps(report, fakeTokenSource{}, 1000)
	var out, errOut bytes.Buffer
	if got := d.doctor(&out, &errOut, nil); got != 0 {
		t.Fatalf("doctor() = %d, want 0; stderr=%q", got, errOut.String())
	}
	if !strings.Contains(out.String(), "/run/sshakku/agent.sock") {
		t.Errorf("output = %q, want the formatted report", out.String())
	}
}

// TestDoctorTestBackend covers the --test-backend branch: it runs the secret
// backend probe against an injected in-memory backend and reports PASS. The
// backend comes from depsReturning; the doctor-specific seams are layered on top
// so the report and euid stay synthetic too.
func TestDoctorTestBackend(t *testing.T) {
	tempRuntimeEnv(t)
	d := depsReturning(newMemoryBackend())
	d.gather = func(paths.Env, paths.Layout) diagnose.Report { return diagnose.Report{} }
	d.tokenSource = fakeTokenSource{}
	d.geteuid = func() int { return 1000 }
	var out, errOut bytes.Buffer
	if got := d.doctor(&out, &errOut, []string{"--test-backend"}); got != 0 {
		t.Fatalf("doctor(--test-backend) = %d, want 0; stderr=%q", got, errOut.String())
	}
	if !strings.Contains(out.String(), "backend test: PASS") {
		t.Errorf("output = %q, want a backend PASS", out.String())
	}
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
		d.ensurer = fakeEnsurer{res: agent.EnsureResult{LiveSock: layout.AgentSock}}
		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 0 {
			t.Fatalf("doctor(--fix) = %d, want 0; stderr=%q", got, errOut.String())
		}
		if strings.Contains(out.String(), "export SSH_AUTH_SOCK") {
			t.Errorf("output = %q, want no re-export hint when already wired", out.String())
		}
	})

	t.Run("unwired shell prints the re-export hint", func(t *testing.T) {
		tempRuntimeEnv(t)
		report := diagnose.Report{EnvSock: "/somewhere/else.sock"}
		d := doctorDeps(report, fakeTokenSource{}, 1000)
		d.ensurer = fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}}
		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 0 {
			t.Fatalf("doctor(--fix) = %d, want 0; stderr=%q", got, errOut.String())
		}
		if !strings.Contains(out.String(), "export SSH_AUTH_SOCK=") {
			t.Errorf("output = %q, want the re-export hint", out.String())
		}
	})

	t.Run("uncreatable layout returns 1", func(t *testing.T) {
		home := tempRuntimeEnv(t)
		if err := os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir"), 0o600); err != nil {
			t.Fatalf("seed ~/.config file: %v", err)
		}
		d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 1 {
			t.Errorf("doctor(--fix, uncreatable layout) = %d, want 1", got)
		}
	})

	t.Run("ensure failure propagates its code", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
		d.ensurer = fakeEnsurer{err: errors.New("boom")}
		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 1 {
			t.Errorf("doctor(--fix, ensure fails) = %d, want 1", got)
		}
	})
}

// TestDoctorCrossUser covers the root-only cross-user dispatch and body against
// fakes: euid 0 lets crossUserGuard pass, and the fake token source stands in
// for reading the target's keyring. A token read error returns 1; a successful
// read reports on the target's session and returns 0.
func TestDoctorCrossUser(t *testing.T) {
	if _, err := lookupUser("nobody"); err != nil {
		t.Skip("no 'nobody' user on this host")
	}

	t.Run("token read failure returns 1", func(t *testing.T) {
		d := doctorDeps(diagnose.Report{}, fakeTokenSource{err: errors.New("keyring boom")}, 0)
		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--user", "nobody"}); got != 1 {
			t.Fatalf("doctor(--user nobody, root, token fails) = %d, want 1", got)
		}
		if !strings.Contains(errOut.String(), "keyring boom") {
			t.Errorf("stderr = %q, want the token-read error", errOut.String())
		}
	})

	t.Run("successful read reports the target session", func(t *testing.T) {
		d := doctorDeps(diagnose.Report{}, fakeTokenSource{token: "tok"}, 0)
		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--user", "nobody"}); got != 0 {
			t.Fatalf("doctor(--user nobody, root) = %d, want 0; stderr=%q", got, errOut.String())
		}
		if !strings.Contains(out.String(), "diagnosing uid") {
			t.Errorf("output = %q, want a cross-user diagnosis header", out.String())
		}
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
	report := gatherReport(env, layout)
	if report.FixedSock != layout.AgentSock {
		t.Errorf("report.FixedSock = %q, want %q", report.FixedSock, layout.AgentSock)
	}
}
