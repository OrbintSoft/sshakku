package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (w *walletSpy) view(context.Context, config.Settings) diagnose.WalletView {
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

func (m *makerSpy) make(context.Context, config.Settings) (string, error) {
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
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())

		assert.NotContains(t, compartmentLine(t, out.String()), "found",
			"a compartment that is not there must not be called found")
		assert.Contains(t, out.String(), "--fix", "and the report must name what would make it")
	})

	t.Run("--fix makes it and says what it made", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{
			compartmentView(answering, true),
			compartmentView(secretServiceLook{running: true, collectionFound: true}, true),
		}}
		maker := &makerSpy{made: theCompartment}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())

		assert.Equal(t, 1, maker.calls, "--fix must make exactly what the report offered, once")
		assert.Contains(t, fixSection(t, out.String()), theCompartment, "and say what it made")
	})

	t.Run("the report after --fix lists the compartment as present", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{
			compartmentView(answering, true),
			compartmentView(secretServiceLook{running: true, collectionFound: true}, true),
		}}
		d := compartmentDeps(t, wallet, &makerSpy{made: theCompartment})

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())

		after := afterReport(t, out.String())
		require.Contains(t, after, "compartment",
			"a report that says nothing about the wallet cannot show anything wallet-shaped was repaired")
		assert.Contains(t, after, theCompartment, "and it must list the compartment that is now there")
	})

	t.Run("with no screen it says it cannot, and the wallet is left as it was", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{compartmentView(answering, false)}}
		maker := &makerSpy{made: theCompartment}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())

		assert.Zero(t, maker.calls, "there is no screen to answer a dialog on, so nothing may be attempted")
		fix := fixSection(t, out.String())
		assert.Contains(t, fix, "compartment", "--fix must say what it could not make")
		assert.Contains(t, fix, "screen", "and what making it would take")
		assert.Contains(t, out.String(), "\nafter:\n",
			"a session that could not be repaired must still get its report")
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
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())
		assert.NotContains(t, fixSection(t, out.String()), "made",
			"a system with nothing of the kind to create must not be told something was created")
	})

	t.Run("a wallet that refuses says so, and the report still comes back", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{compartmentView(answering, true)}}
		maker := &makerSpy{err: errors.New("the dialog was dismissed")}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		// Not zero: --fix was asked to repair, it tried, and the wallet refused.
		// A caller that only reads the exit code would otherwise be told the
		// repair succeeded.
		assert.Equalf(t, 1, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}),
			"a repair that was attempted and refused must not exit as though it worked; stderr=%q", errOut.String())

		assert.Contains(t, out.String()+errOut.String(), "the dialog was dismissed",
			"and the reason must reach the user")
		assert.Contains(t, out.String(), "\nafter:\n", "one wallet that refused must not take the whole report with it")
	})

	t.Run("a compartment that is already there is left exactly as it is", func(t *testing.T) {
		wallet := &walletSpy{views: []diagnose.WalletView{
			compartmentView(secretServiceLook{running: true, collectionFound: true}, true),
		}}
		maker := &makerSpy{made: theCompartment}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())

		assert.Zero(t, maker.calls,
			"making a compartment the wallet already holds is asking it for a second one nobody wanted")
	})

	t.Run("a compartment nobody could ask about is neither made nor pronounced on", func(t *testing.T) {
		// Nothing was answering, so whether the compartment is there was never
		// established. Acting on that is how a wallet ends up holding something
		// nobody asked for; saying --fix cannot provide it states as fact
		// something about a machine nothing was learned from.
		wallet := &walletSpy{views: []diagnose.WalletView{compartmentView(secretServiceLook{}, true)}}
		maker := &makerSpy{made: theCompartment}
		d := compartmentDeps(t, wallet, maker)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}), "--fix; stderr=%q", errOut.String())

		assert.Zero(t, maker.calls, "nothing may be made on the strength of a question nobody could ask")
		assert.NotContains(t, fixSection(t, out.String()), "not something --fix can provide",
			"nor may anything be declared impossible about a wallet that was never reached")
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
	require.FailNowf(t, "the report is missing the line about the compartment", "no compartment line in:\n%s", report)
	return ""
}

// fixSection is what --fix printed about its own work: everything from the
// self-heal header to the report that follows it.
func fixSection(t *testing.T, out string) string {
	t.Helper()
	_, after, ok := strings.Cut(out, "── applying self-heal ──")
	require.Truef(t, ok, "--fix printed nothing about its own work:\n%s", out)
	section, _, _ := strings.Cut(after, "\nafter:\n")
	return section
}

// afterReport is the report --fix prints once it has finished.
func afterReport(t *testing.T, out string) string {
	t.Helper()
	_, after, ok := strings.Cut(out, "\nafter:\n")
	require.Truef(t, ok, "--fix printed no report once it had finished:\n%s", out)
	return after
}
