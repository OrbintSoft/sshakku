package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/handoff"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// askpass answers an SSH_ASKPASS request. The proactive key-loading path
// stashes the passphrase via the OS-appropriate handoff and points us at it
// via $SSHAKKU_HANDOFF_TOKEN; with a token we serve that one-shot stash.
// Without one we are the reactive broker for an interactive ssh whose key has
// expired, and answer the prompt in args from the wallet (or the terminal).
func (d deps) askpass(ctx context.Context, stdout io.Writer, args []string) int {
	if os.Getenv(handoff.EnvToken) != "" {
		return d.askpassFromHandoff(ctx, stdout)
	}
	return d.askpassBroker(ctx, stdout, args)
}

// askpassBroker answers ssh's passphrase or confirmation prompt: a key passphrase
// comes from the wallet (or the terminal on a miss), other prompts pass through to
// the terminal. Only the reply goes to stdout; diagnostics go to the session log.
// It reads the config file for the wallet-store policy, so a miss-then-store
// refill honours the same include/exclude rules as load-keys.
func (d deps) askpassBroker(ctx context.Context, stdout io.Writer, args []string) int {
	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir)
	log := sessionlog.New(layout.LogFile)
	settings := loadSettings(layout, "askpass", log)
	secret, closeSecret := d.newSecret(ctx, currentUser(), log, settings)
	defer closeSecret()
	broker := keys.Broker{
		Secret: secret,
		TTY:    d.tty,
		Log:    log,
		Config: keys.Config{WalletStore: settings.StoresWallet, ServicePrefix: settings.ServicePrefix},
	}
	reply, ok := broker.Answer(ctx, strings.Join(args, " "))
	if !ok {
		return 1
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", reply); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: write reply: %v", err))
		return 1
	}
	return 0
}

// askpassFromHandoff reads the passphrase the loader stashed via the
// OS-appropriate handoff, identified by the token in $SSHAKKU_HANDOFF_TOKEN,
// prints it on stdout for ssh-add, and invalidates the one-shot stash. The
// passphrase never touches stderr or argv; only the handoff token crosses the
// environment. Diagnostics go to the session log alone, so the success path
// stays silent.
func (d deps) askpassFromHandoff(ctx context.Context, stdout io.Writer) int {
	log := sessionlog.New(paths.Resolve(paths.FromOS(), paths.ProbeDir).LogFile)

	token := os.Getenv(handoff.EnvToken)
	if token == "" {
		_ = log.Log("ERROR", "askpass: missing handoff token")
		return 1
	}

	pass, err := d.fetchHandoff(ctx, token)
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: fetch handoff token …%s: %v", tail(token, 3), err))
		return 1
	}
	if len(pass) == 0 {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: empty passphrase for handoff token …%s", tail(token, 3)))
		return 1
	}

	// ssh-add reads the passphrase from stdout and strips the trailing newline.
	if _, err := fmt.Fprintf(stdout, "%s\n", pass); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: write passphrase: %v", err))
		return 1
	}
	_ = log.Log("INFO", fmt.Sprintf("askpass: provided passphrase for handoff token …%s", tail(token, 3)))
	return 0
}

// tail returns the last n characters of s, for logging a handoff token without
// recording it in full.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// askpassEnv prints the export lines that route this shell's ssh passphrase
// prompts through sshakku's wallet-aware broker, so a key that expires from the
// agent is refilled from the wallet without a terminal prompt. Every session
// gets them, graphical or not: reading the wallet needs no display, and on a
// wallet miss the broker prompts on the terminal exactly as ssh would have, so
// there is nothing a headless session gains by being left unwired. The login
// entrypoint evals it in every login shell, interactive or not, since it only
// ever prints export lines. --shell says which language to print them in (see
// shell.FromArgs).
func (d deps) askpassEnv(stdout, stderr io.Writer, args []string) int {
	dialect, err := shell.FromArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: askpass-env: %v\n", err)
		return 2
	}
	self, err := d.self()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	if _, err := io.WriteString(stdout, askpassExports(dialect, self)); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// askpassExports returns the lines, in the caller's own shell language, that
// put ssh's SSH_ASKPASS in the environment pointing at the wallet-aware broker
// installed beside self. REQUIRE=force is what routes
// every prompt to the broker: `prefer` asks OpenSSH to favour the helper but
// still ignores it when DISPLAY is unset, which is most terminal sessions and a
// Mac without an X server. The broker keeps the terminal as its own fallback,
// so forcing it costs a session nothing it had before.
//
// Both lines land in every login shell and stay there, which is why neither may
// say anything about a particular command: they describe where ssh should go for
// a passphrase, and nothing else in the session is affected by them.
func askpassExports(dialect shell.Dialect, self string) string {
	return dialect.SetEnv("SSH_ASKPASS", askpassProg(self)) +
		dialect.SetEnv("SSH_ASKPASS_REQUIRE", "force")
}
