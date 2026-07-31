package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/giveup"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// loadKeys adds the user's ~/.ssh keys to the agent: it skips keys already loaded
// and, for the rest, pulls each passphrase from the secret store (or prompts) and
// hands it to ssh-add out of band. The login entrypoint calls it only in
// interactive shells. SSH_ASKPASS points at this very binary, which ssh-add
// re-execs to fetch the stashed passphrase. The success path is silent;
// problems go to the session log (and stderr for a hard failure).
func (d deps) loadKeys(stderr io.Writer) int {
	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	log := sessionlog.New(layout.LogFile)

	self, err := d.self()
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("load-keys: locate self: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}

	settings := loadSettings(layout, "load-keys", log)

	var giveupStore keys.GiveupStore
	if !settings.NoGiveup {
		giveupStore = giveup.Store{
			Dir: filepath.Join(filepath.Dir(layout.AgentSock), "giveup"),
			TTL: settings.GiveupTTL,
		}
	}
	keyStateStore := keystate.Store{Dir: keystateDir(layout)}

	var notifier keys.Notifier
	if !settings.Quiet {
		notifier = stderrNotifier{w: stderr}
	}

	runner := keys.ExecRunner{Timeout: settings.CommandTimeout}
	kdialog := keys.KDialogPrompter{Runner: runner, Timeout: settings.InteractiveTimeout}
	// The vault is always consulted first regardless of which of these is
	// picked (see Loader.loadViaVaultThenPrompt); this only chooses how to
	// ask when it misses — kdialog when a graphical session is usable,
	// otherwise the terminal, which needs no external binary.
	var prompter keys.Prompter = keys.TTYPrompter{}
	if d.guiAvailable() {
		prompter = kdialog
	}

	secret, closeSecret := d.newSecret(currentUser(), log, settings)
	defer closeSecret()

	loader := keys.Loader{
		Keys:     keys.Enumerator{Dir: filepath.Join(env.Home, ".ssh")},
		Runner:   runner,
		Secret:   secret,
		Prompt:   prompter,
		Adder:    keys.ExecKeyAdder{AskpassProg: self, KeyLifetime: settings.KeyLifetime},
		Log:      log,
		Notify:   notifier,
		Giveup:   giveupStore,
		KeyState: keyStateStore,
		Config: keys.Config{
			MaxAttempts: settings.MaxAttempts,
			WalletStore: settings.StoresWallet,
			AutoLoad:    settings.AutoLoads,
			KeyLifetime: settings.KeyLifetime,
		},
	}
	if err := loader.LoadKeys(); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("load-keys: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// stderrNotifier surfaces a user-facing notice to the terminal of the
// interactive shell that ran load-keys; $SSHAKKU_QUIET suppresses it.
type stderrNotifier struct{ w io.Writer }

func (n stderrNotifier) Notify(message string) {
	_, _ = fmt.Fprintf(n.w, "sshakku: %s\n", message)
}
