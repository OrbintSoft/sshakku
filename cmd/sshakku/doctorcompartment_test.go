package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
)

// theCompartment is the name the cases below watch for. Any name would do; a
// distinctive one makes "the report said what it made" impossible to satisfy by
// accident from some other line.
const theCompartment = "the-compartment"

// walletSpy answers with a different view each time it is asked, so a case can
// describe the machine before --fix and the machine after it — which is the
// only way "the report that comes afterwards" can be about anything. The last
// view is repeated once the list runs out.
type walletSpy struct {
	views []diagnose.WalletView
	calls int
}

func (w *walletSpy) view(config.Settings) diagnose.WalletView {
	view := w.views[min(w.calls, len(w.views)-1)]
	w.calls++
	return view
}

// compartmentView is a wallet whose only piece is the compartment, described by
// the code that decides what to say about one rather than by a sentence written
// here: what a case asserts is then what a user would actually read.
func compartmentView(look secretServiceLook, hasScreen bool) diagnose.WalletView {
	return diagnose.WalletView{
		// Whichever wallet this system opens by default: the cases below are
		// about a compartment, and naming the one wallet that has compartments
		// would put them behind a platform they do not belong to.
		Backend:      config.DefaultSecretBackend(),
		Requirements: []diagnose.Requirement{compartmentRequirement(theCompartment, look, hasScreen)},
	}
}

// makerSpy stands in for the act of creating the compartment, recording whether
// it was asked for. What it replaces is the wallet, not the decision: whether
// anything is created at all is what these cases judge.
type makerSpy struct {
	made  string
	err   error
	calls int
}

func (m *makerSpy) make(config.Settings) (string, error) {
	m.calls++
	return m.made, m.err
}

// compartmentDeps builds a doctor whose machine is the one described: a wallet
// answering, the compartment not there, and a screen or no screen.
func compartmentDeps(t *testing.T, wallet *walletSpy, maker *makerSpy) deps {
	t.Helper()
	tempRuntimeEnv(t)
	d := doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
	d.wallet = wallet.view
	d.makeCompartment = maker.make
	return d
}

// TestDoctorMakesTheCompartment verifies F42: the compartment a wallet keeps
// SSHakku's passphrases in is something SSHakku makes for you, and where it
// cannot be made you are told so and the wallet is left as it was found.
//
// Each case is one sentence of F42's "how you can tell", and the wallet is
// described through the report's own words rather than through the code that
// would create anything.
func TestDoctorMakesTheCompartment(t *testing.T) {
	answering := secretServiceLook{running: true}

	t.Run("the report says it is not there, and names --fix as what makes it", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{compartmentView(answering, true)}}
		d := compartmentDeps(t, wallet, &makerSpy{})

		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, nil); got != 0 {
			t.Fatalf("doctor = %d, want 0; stderr=%q", got, errOut.String())
		}

		if line := compartmentLine(t, out.String()); strings.Contains(line, "found") {
			t.Errorf("the report calls a compartment that is not there found:\n%s", line)
		}
		if !strings.Contains(out.String(), "--fix") {
			t.Errorf("the report never names --fix as what makes the compartment:\n%s", out.String())
		}
	})

	t.Run("--fix makes it and says what it made", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{
			compartmentView(answering, true),
			compartmentView(secretServiceLook{running: true, collectionFound: true}, true),
		}}
		maker := &makerSpy{made: theCompartment}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 0 {
			t.Fatalf("doctor --fix = %d, want 0; stderr=%q", got, errOut.String())
		}

		if maker.calls != 1 {
			t.Errorf("the compartment was asked for %d times, want once: --fix did not make what the report offered", maker.calls)
		}
		if !strings.Contains(fixSection(t, out.String()), theCompartment) {
			t.Errorf("--fix never says what it made:\n%s", out.String())
		}
	})

	t.Run("the report after --fix lists the compartment as present", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{
			compartmentView(answering, true),
			compartmentView(secretServiceLook{running: true, collectionFound: true}, true),
		}}
		d := compartmentDeps(t, wallet, &makerSpy{made: theCompartment})

		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 0 {
			t.Fatalf("doctor --fix = %d, want 0; stderr=%q", got, errOut.String())
		}

		after := afterReport(t, out.String())
		if !strings.Contains(after, "compartment") {
			t.Fatalf("the report --fix prints afterwards says nothing about the wallet, so nothing wallet-shaped can be shown to have been repaired:\n%s", after)
		}
		if !strings.Contains(after, theCompartment) {
			t.Errorf("the report afterwards does not list the compartment as present:\n%s", after)
		}
	})

	t.Run("with no screen it says it cannot, and the wallet is left as it was", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{compartmentView(answering, false)}}
		maker := &makerSpy{made: theCompartment}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 0 {
			t.Fatalf("doctor --fix = %d, want 0; stderr=%q", got, errOut.String())
		}

		if maker.calls != 0 {
			t.Errorf("the compartment was made %d times in a session that has no screen to make it on", maker.calls)
		}
		fix := fixSection(t, out.String())
		if !strings.Contains(fix, "compartment") {
			t.Errorf("--fix says nothing about the compartment it could not make:\n%s", fix)
		}
		if !strings.Contains(fix, "screen") {
			t.Errorf("--fix does not say what making the compartment would take:\n%s", fix)
		}
		if !strings.Contains(out.String(), "\nafter:\n") {
			t.Error("the report did not come back at all from a session that could not be repaired")
		}
	})

	t.Run("a wallet with no compartment to make is left alone", func(t *testing.T) {
		// The answer for a system whose wallet keeps SSHakku's entries wherever
		// the account already has them: there is nothing of the kind to create,
		// so --fix creates nothing and claims nothing. Checked from here so that
		// the answer given on the other platform is not one only that platform
		// can see.
		wallet := &walletSpy{views: []diagnose.WalletView{compartmentView(answering, true)}}
		d := compartmentDeps(t, wallet, &makerSpy{})
		d.makeCompartment = nil

		var out, errOut bytes.Buffer
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 0 {
			t.Fatalf("doctor --fix = %d, want 0; stderr=%q", got, errOut.String())
		}
		if strings.Contains(fixSection(t, out.String()), "made") {
			t.Errorf("--fix claims to have made something on a system with nothing to make:\n%s", out.String())
		}
	})

	t.Run("a wallet that refuses says so, and the report still comes back", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{compartmentView(answering, true)}}
		maker := &makerSpy{err: errors.New("the dialog was dismissed")}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		// Not zero: --fix was asked to repair, it tried, and the wallet refused.
		// A caller that only reads the exit code would otherwise be told the
		// repair succeeded.
		if got := d.doctor(&out, &errOut, []string{"--fix"}); got != 1 {
			t.Fatalf("doctor --fix = %d, want 1; stderr=%q", got, errOut.String())
		}

		if !strings.Contains(out.String()+errOut.String(), "the dialog was dismissed") {
			t.Errorf("a compartment that could not be made is not reported:\nstdout=%s\nstderr=%s", out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), "\nafter:\n") {
			t.Error("one wallet that refused took the whole report with it")
		}
	})
}

// compartmentLine is the one line of the report a user reads to learn whether
// the compartment is there. Taken from the printed report rather than from the
// requirement behind it, because the word that appears on it is the whole of
// what is being judged.
func compartmentLine(t *testing.T, report string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		if strings.Contains(line, "compartment:") {
			return line
		}
	}
	t.Fatalf("no compartment line in:\n%s", report)
	return ""
}

// fixSection is what --fix printed about its own work: everything from the
// self-heal header to the report that follows it.
func fixSection(t *testing.T, out string) string {
	t.Helper()
	_, after, ok := strings.Cut(out, "── applying self-heal ──")
	if !ok {
		t.Fatalf("no self-heal section in:\n%s", out)
	}
	section, _, _ := strings.Cut(after, "\nafter:\n")
	return section
}

// afterReport is the report --fix prints once it has finished.
func afterReport(t *testing.T, out string) string {
	t.Helper()
	_, after, ok := strings.Cut(out, "\nafter:\n")
	if !ok {
		t.Fatalf("no report after the self-heal in:\n%s", out)
	}
	return after
}
