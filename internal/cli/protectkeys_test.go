package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F69's acting half, through the command a user runs rather than
// through the functions under it.
//
// What is substituted here is what belongs to a system: whether a path is
// encrypted to one account, and what encrypting one does. That seam is driven
// against the real Encrypting File System in the protect package. What is not
// substituted is the decision under test — which paths reach it, what is said
// before they do, and whether anything is asked first — and the run goes
// through the dispatcher, so what a user types is what these tests type.

// aProtector stands for what a system does to a path: it records what it was
// asked to cover and answers afterwards as that system would.
type aProtector struct {
	// covered is every path handed to the acting call, in order — the record
	// that says what a run actually changed, as against what it printed.
	covered []string
	// protected is what the system says about a path when it is asked. A path
	// nobody has mentioned is one that is not protected, which is what a key
	// file written a moment ago is.
	protected map[string]bool
}

func newProtector() *aProtector { return &aProtector{protected: map[string]bool{}} }

func (p *aProtector) seam() keyProtector {
	return keyProtector{
		scheme: "EFS",
		look: func(path string) (*bool, error) {
			answer := p.protected[path]
			return &answer, nil
		},
		apply: func(path string) error {
			p.covered = append(p.covered, path)
			p.protected[path] = true
			return nil
		},
	}
}

// aKeyDirectory points this process's environment at a fresh home, puts a key
// directory of the given name in it holding the named files, and returns that
// directory. A name other than the default is written into the configuration,
// the way a user who moved their keys would have it.
func aKeyDirectory(t *testing.T, dirName string, files ...string) string {
	t.Helper()
	home := tempRuntimeEnv(t)
	if dirName != ".ssh" {
		writeConfig(t, home, "config.toml", "key_dir = "+strconv.Quote(dirName)+"\n")
	}
	dir := filepath.Join(home, dirName)
	require.NoError(t, os.MkdirAll(dir, 0o700), "create the key directory")
	for _, name := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("not really a key"), 0o600), name)
	}
	return dir
}

// runProtectKeys runs the command through the dispatcher with the given answer
// waiting on standard input, and hands back what the user would see.
func runProtectKeys(t *testing.T, p *aProtector, answer string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	d := realDeps()
	d.keyProtection = p.seam()
	d.stdin = strings.NewReader(answer)
	var out, errOut bytes.Buffer
	code = d.run(t.Context(), &out, &errOut, append([]string{"protect-keys"}, args...))
	return code, out.String(), errOut.String()
}

// TestADryRunNamesTheKeysAndChangesNothing. Finding out what this would do has
// to cost nothing, or nobody runs it on the keys they actually depend on.
func TestADryRunNamesTheKeysAndChangesNothing(t *testing.T) {
	dir := aKeyDirectory(t, ".ssh", "id_ed25519")
	p := newProtector()

	code, out, errOut := runProtectKeys(t, p, "", "--dry-run")

	require.Zerof(t, code, "a dry run reports what it would do and succeeds; stderr=%q", errOut)
	assert.Contains(t, out, filepath.Join(dir, "id_ed25519"), "the key it would protect is named")
	assert.Empty(t, p.covered, "and nothing at all is handed to the call that would change it")
}

// TestTheKeysAreProtectedAndAuthorizedKeysIsNot is the ordinary run. That file
// holds public keys, so there is nothing in it to protect, and an SSH server
// reads it as the system rather than as the account.
func TestTheKeysAreProtectedAndAuthorizedKeysIsNot(t *testing.T) {
	dir := aKeyDirectory(t, ".ssh", "id_ed25519", "authorized_keys")
	p := newProtector()

	code, out, errOut := runProtectKeys(t, p, "")

	require.Zerof(t, code, "protecting the keys succeeds; stderr=%q", errOut)
	assert.Equal(t, []string{filepath.Join(dir, "id_ed25519")}, p.covered,
		"the private key, and nothing else — the directory is not covered unless it is asked for")
	assert.NotContains(t, out, "authorized_keys", "nor is it offered as something that could be")
}

// TestAKeyAlreadyProtectedIsNotProtectedAgain. A run over keys that are already
// covered says so rather than reporting work it did not do.
func TestAKeyAlreadyProtectedIsNotProtectedAgain(t *testing.T) {
	dir := aKeyDirectory(t, ".ssh", "id_ed25519")
	p := newProtector()
	p.protected[filepath.Join(dir, "id_ed25519")] = true

	code, out, errOut := runProtectKeys(t, p, "")

	require.Zerof(t, code, "there being nothing to do is not a failure; stderr=%q", errOut)
	assert.Empty(t, p.covered, "nothing is handed to the acting call")
	assert.Contains(t, out, "already protected", "and the report says why there was nothing to do")
}

// TestCoveringTheSSHDirectorySaysWhatItCostsFirst. Covering the directory is
// what makes the next key generated there born protected — and where that
// directory is the one an SSH server reads, a file created in it afterwards is
// born encrypted too, including the authorized_keys a later ssh-copy-id writes.
func TestCoveringTheSSHDirectorySaysWhatItCostsFirst(t *testing.T) {
	dir := aKeyDirectory(t, ".ssh", "id_ed25519")
	p := newProtector()

	code, out, errOut := runProtectKeys(t, p, "", "--directory")

	require.Zerof(t, code, "asked for, with no authorized_keys there, it proceeds; stderr=%q", errOut)
	assert.Contains(t, out, "authorized_keys", "what would be born encrypted is named")
	assert.Contains(t, out, "sshakku move-keys", "and so is the command that avoids the whole thing")
	assert.Equal(t, dir, p.covered[0], "the directory first, so the next key born there is covered")
}

// TestAnAuthorizedKeysInTheDirectoryIsNotWarnedAboutButAskedAbout. Its being
// there is what says an SSH server really does read this directory, so a
// warning printed on the way past is not enough.
func TestAnAuthorizedKeysInTheDirectoryIsNotWarnedAboutButAskedAbout(t *testing.T) {
	t.Run("an answer that is not yes changes nothing", func(t *testing.T) {
		aKeyDirectory(t, ".ssh", "id_ed25519", "authorized_keys")
		p := newProtector()

		code, out, _ := runProtectKeys(t, p, "no\n", "--directory")

		assert.NotZero(t, code, "the keys are not protected, and the exit code says so")
		assert.Empty(t, p.covered, "nothing is covered — not the directory, and not the keys either")
		assert.Contains(t, out, "yes", "the answer it was waiting for is named")
	})

	t.Run("nothing on standard input is not a yes", func(t *testing.T) {
		aKeyDirectory(t, ".ssh", "id_ed25519", "authorized_keys")
		p := newProtector()

		code, out, _ := runProtectKeys(t, p, "", "--directory")

		assert.NotZero(t, code, "a question nobody was there to answer was not answered")
		assert.Empty(t, p.covered)
		assert.Contains(t, out, "yes", "it was asked all the same, rather than decided quietly")
	})

	t.Run("confirmed in as many words, it goes ahead", func(t *testing.T) {
		dir := aKeyDirectory(t, ".ssh", "id_ed25519", "authorized_keys")
		p := newProtector()

		code, _, errOut := runProtectKeys(t, p, "yes\n", "--directory")

		require.Zerof(t, code, "confirmed, the run succeeds; stderr=%q", errOut)
		assert.Equal(t, []string{dir, filepath.Join(dir, "id_ed25519")}, p.covered)
		assert.NotContains(t, p.covered, filepath.Join(dir, "authorized_keys"),
			"and that file is still not among the things it covers")
	})
}

// TestTheWarningDependsOnWhetherAnAuthorizedKeysIsAlreadyThere. Encrypting a
// directory changes what files created in it afterwards are born as, and
// nothing else — measured on the real filesystem, which is what settles this:
// an authorized_keys already sitting there survives appends and edits made in
// place, and only a file deleted and recreated comes back encrypted. So the two
// situations carry different risks, and one warning covering both would
// misdescribe whichever it was not written for.
func TestTheWarningDependsOnWhetherAnAuthorizedKeysIsAlreadyThere(t *testing.T) {
	t.Run("none there yet, so the first one added is the problem", func(t *testing.T) {
		aKeyDirectory(t, ".ssh", "id_ed25519")

		_, out, _ := runProtectKeys(t, newProtector(), "", "--directory", "--dry-run")

		assert.Contains(t, out, "no authorized_keys here yet")
		assert.NotContains(t, out, "deleted and recreated", "there is nothing there to delete")
	})

	t.Run("one is there, and it goes on working", func(t *testing.T) {
		aKeyDirectory(t, ".ssh", "id_ed25519", "authorized_keys")

		_, out, _ := runProtectKeys(t, newProtector(), "", "--directory", "--dry-run")

		assert.Contains(t, out, "is not affected", "the file that is there keeps working, and saying so is honest")
		assert.Contains(t, out, "deleted and recreated", "and what would actually break it is named")
	})
}

// TestADryRunNeverAsks. It is the run that changes nothing, so there is nothing
// to ask about — and a prompt on a run somebody expected to be silent is how a
// scripted dry run hangs.
func TestADryRunNeverAsks(t *testing.T) {
	aKeyDirectory(t, ".ssh", "id_ed25519", "authorized_keys")
	p := newProtector()

	code, out, errOut := runProtectKeys(t, p, "yes\n", "--directory", "--dry-run")

	require.Zerof(t, code, "a dry run succeeds whatever it would have had to ask; stderr=%q", errOut)
	assert.Empty(t, p.covered, "with a yes waiting on standard input, it still changes nothing")
	assert.Contains(t, out, "authorized_keys", "what a real run would stop over is still said")
}

// TestADirectoryNoServerReadsIsCoveredWithoutCeremony. The warning is about
// what an SSH server does with the directory, not about covering directories,
// and a user who took the advice must not be lectured for having taken it.
func TestADirectoryNoServerReadsIsCoveredWithoutCeremony(t *testing.T) {
	dir := aKeyDirectory(t, "keys", "id_ed25519")
	p := newProtector()

	code, out, errOut := runProtectKeys(t, p, "", "--directory")

	require.Zerof(t, code, "covering it succeeds; stderr=%q", errOut)
	assert.Equal(t, []string{dir, filepath.Join(dir, "id_ed25519")}, p.covered)
	assert.NotContains(t, out, "WARNING", "the keys are already where that advice leads")
}

// TestASystemWithNoSchemeSaysSoAndChangesNothing. F69 ends where the system
// does: what SSHakku cannot do here, it does not pretend to have done.
func TestASystemWithNoSchemeSaysSoAndChangesNothing(t *testing.T) {
	aKeyDirectory(t, ".ssh", "id_ed25519")
	p := newProtector()
	d := realDeps()
	d.keyProtection = keyProtector{}
	d.stdin = strings.NewReader("")

	var out, errOut bytes.Buffer
	code := d.run(t.Context(), &out, &errOut, []string{"protect-keys"})

	assert.NotZero(t, code, "nothing was protected, and the exit code does not say otherwise")
	assert.Contains(t, errOut.String(), "this system", "what is absent is the system's, not the command's")
	assert.NotContains(t, errOut.String(), "unknown", "the command is known here; what it needs is not")
	assert.Empty(t, p.covered)
}

// TestAnArgumentItDoesNotKnowIsAUsageError, rather than a run that quietly does
// something other than what was typed.
func TestAnArgumentItDoesNotKnowIsAUsageError(t *testing.T) {
	aKeyDirectory(t, ".ssh", "id_ed25519")
	p := newProtector()

	code, _, errOut := runProtectKeys(t, p, "", "--everything")

	assert.Equal(t, 2, code, "a usage error, as every other command answers one")
	assert.Contains(t, errOut, "--everything")
	assert.Empty(t, p.covered)
}

// TestNoKeysToProtectSaysSoRatherThanSucceedingSilently, and names where it
// looked: an empty answer about the wrong directory is how somebody concludes
// their keys are covered when nothing of theirs was ever read.
func TestNoKeysToProtectSaysSoRatherThanSucceedingSilently(t *testing.T) {
	dir := aKeyDirectory(t, ".ssh")
	p := newProtector()

	code, out, errOut := runProtectKeys(t, p, "")

	require.Zerof(t, code, "having nothing to do is not a failure; stderr=%q", errOut)
	assert.Contains(t, out, "No keys")
	assert.Contains(t, out, dir, "and where it looked")
	assert.Empty(t, p.covered)
}

// TestAKeyDirectoryThatCannotBeReadIsReported rather than read as an account
// with no keys: "there is nothing to protect" and "I could not look" are
// different answers, and only one of them means everything is in order.
func TestAKeyDirectoryThatCannotBeReadIsReported(t *testing.T) {
	home := tempRuntimeEnv(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh"), nil, 0o600))
	p := newProtector()

	code, out, errOut := runProtectKeys(t, p, "")

	assert.NotZero(t, code)
	assert.NotContains(t, out, "No keys", "an unreadable directory is not an empty one")
	assert.Contains(t, errOut, ".ssh", "the directory it could not read is named")
	assert.Empty(t, p.covered)
}

// TestAKeyThatCouldNotBeProtectedIsNamed. The run reports what the filesystem
// says afterwards rather than what the calls returned, and a key it could not
// cover has to be named: it is the one the user still has to do something
// about, and a count alone does not say which.
func TestAKeyThatCouldNotBeProtectedIsNamed(t *testing.T) {
	dir := aKeyDirectory(t, ".ssh", "id_ed25519")
	d := realDeps()
	d.stdin = strings.NewReader("")
	d.keyProtection = keyProtector{
		scheme: "EFS",
		look: func(string) (*bool, error) {
			no := false
			return &no, nil
		},
		apply: func(string) error { return assert.AnError },
	}

	var out, errOut bytes.Buffer
	code := d.run(t.Context(), &out, &errOut, []string{"protect-keys"})

	assert.NotZero(t, code, "a key that is not protected is not a successful run")
	assert.Contains(t, errOut.String(), filepath.Join(dir, "id_ed25519"), "the key is named")
	assert.Contains(t, out.String(), "0 of 1", "and the count says how much of the job was done")
}

// TestNobodyToAskIsNotAYes covers a run with no standard input at all, as
// against one where somebody is there and says nothing. Both are no: a default
// of yes would be the same as never having asked, and what is at stake is key
// logins into the machine.
func TestNobodyToAskIsNotAYes(t *testing.T) {
	var out bytes.Buffer

	answered := protectKeysConfirmed(nil, &out)

	assert.False(t, answered, "with nowhere to ask, the answer is no")
	assert.Contains(t, out.String(), "yes", "and the question is still shown, rather than decided quietly")
}

// TestWhatAMachineCanDoAboutAKeyAtRest covers both answers a system can give,
// which no single machine can: the one that has a scheme, and the one that has
// none and must be given no way to ask rather than a way that always refuses.
func TestWhatAMachineCanDoAboutAKeyAtRest(t *testing.T) {
	assert.Equal(t, keyProtector{}, keyProtectionFor(""),
		"a system with no scheme is given nothing to ask with, so nothing is claimed and no key is opened")

	with := keyProtectionFor("EFS")
	assert.Equal(t, "EFS", with.scheme, "a system that has one is named by it")
	assert.NotNil(t, with.look, "and can be asked whether a path is covered")
	assert.NotNil(t, with.apply, "and told to cover one")
}
