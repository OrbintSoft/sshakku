package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/giveup"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// loadKeys adds the user's keys to the agent: it skips keys already loaded
// and, for the rest, pulls each passphrase from the secret store (or prompts) and
// hands it to ssh-add out of band. The login entrypoint calls it only in
// interactive shells. SSH_ASKPASS points at the askpass helper installed beside
// this binary, which ssh-add execs to fetch the stashed passphrase. The success
// path is silent; problems go to the session log (and stderr for a hard
// failure).
func (d deps) loadKeys(ctx context.Context, stderr io.Writer) int {
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
	lifetime, recorded := keyLifetimes(settings.KeyLifetime, agent.KeepsLifetimes())
	if lifetime != settings.KeyLifetime {
		_ = log.Log("INFO", fmt.Sprintf(
			"load-keys: the agent on this system holds no key lifetimes, so keys are added with no expiry"+
				" and the configured %s is not in force", settings.KeyLifetime))
	}

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

	runner := run.ExecRunner{Timeout: settings.CommandTimeout}
	// The vault is always consulted first regardless of which of these is
	// picked (see Loader.loadViaVaultThenPrompt); this only chooses how to
	// ask when it misses — the platform's dialog where there is one, otherwise
	// the terminal, which needs no external binary.
	var prompter prompt.Prompter = prompt.TTYPrompter{}
	if graphical := d.graphicalPrompter(ctx, settings, log); graphical != nil {
		prompter = graphical
	}

	secret, closeSecret := d.newSecret(ctx, currentUser(), log, settings)
	defer closeSecret()

	loader := keys.Loader{
		Keys:     settings.KeyEnumerator(env.Home),
		Runner:   runner,
		Secret:   secret,
		Prompt:   prompter,
		Adder:    keys.ExecKeyAdder{AskpassProg: askpassProg(self), KeyLifetime: lifetime},
		Log:      log,
		Notify:   notifier,
		Giveup:   giveupStore,
		KeyState: keyStateStore,
		Config: keys.Config{
			MaxAttempts:   settings.MaxAttempts,
			WalletStore:   settings.StoresWallet,
			AutoLoad:      settings.AutoLoads,
			KeyLifetime:   recorded,
			ServicePrefix: settings.ServicePrefix,
			OnDismiss:     settings.OnDismiss,
		},
	}
	if err := loader.LoadKeys(ctx); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("load-keys: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// keyLifetimes splits one configured lifetime into the two things it decides:
// what the agent is told to hold the key for, and what the record says the key
// is meant to live for.
//
// Where the agent holds lifetimes the two are the same, and the agent is what
// enforces it. Where it holds none, asking for one is not a smaller version of
// the same thing — the agent refuses the key outright — so the choice is
// between a key that loads without an expiry and no key at all. It loads, and
// the record still keeps the configured lifetime, since that is what the next
// session goes by when it takes a key whose time is up back out of the agent: a
// zero recorded there would mean nothing ever expires. Which system this is
// comes in as the answer it is, so both outcomes stay checkable from either
// machine.
func keyLifetimes(configured time.Duration, agentKeepsLifetimes bool) (told, recorded time.Duration) {
	if agentKeepsLifetimes {
		return configured, configured
	}
	return 0, configured
}

// stderrNotifier surfaces a user-facing notice to the terminal of the
// interactive shell that ran load-keys; $SSHAKKU_QUIET suppresses it.
type stderrNotifier struct{ w io.Writer }

func (n stderrNotifier) Notify(message string) {
	_, _ = fmt.Fprintf(n.w, "sshakku: %s\n", message)
}
