package keys

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// removeTimeout bounds each `ssh-add -d`. It is a plain agent operation with
// nobody to wait for — no passphrase, no dialog — so unlike a key being added
// it must never sit there: this runs at the start of every session, and a stuck
// agent has to become a logged failure rather than a shell that will not open.
const removeTimeout = 5 * time.Second

// AddedKey is one key sshakku put in the agent, as the record of it says.
type AddedKey struct {
	// KeyFile is the file the key was added from, which is what names it to
	// ssh-add again. It is empty for a record that does not say, which is a
	// record nothing here can act on.
	KeyFile string
	// AddedAt is when sshakku put the key in the agent.
	AddedAt time.Time
	// ExpiresAt is when the key stops being one this session may keep; a zero
	// time means it was added with no expiry and never stops.
	ExpiresAt time.Time
}

// AddedKeys is what sshakku recorded of the keys it added, and the only thing
// an Expirer goes by: a key nobody recorded is nobody's to take away.
type AddedKeys interface {
	// Added lists the keys recorded as added, in no particular order.
	Added() ([]AddedKey, error)
	// Forget drops the record for the key in keyfile.
	Forget(keyfile string) error
}

// Expirer takes out of the agent the keys sshakku added whose time is up, for
// a system whose agent expires nothing itself. It is the other half of adding a
// key with no expiry there: the lifetime the user configured is kept by the
// next session to open rather than by the agent, so a key still stops being
// usable — just at the next login rather than at the stroke of its deadline.
//
// Only recorded keys are touched. A key somebody added themselves has no
// record, and its age is not sshakku's business.
type Expirer struct {
	// Records is what sshakku wrote down as it added keys.
	Records AddedKeys
	// Runner runs ssh-add.
	Runner run.Runner
	// Endpoint is handed to ssh-add as SSH_AUTH_SOCK, so the agent it reaches
	// is the one this session was pointed at rather than whatever the
	// environment happens to hold. Empty leaves the environment as it is.
	Endpoint string
	// Log receives one line per key taken out, and per key that could not be.
	Log Logger
	// Now is the clock, overridable in tests; nil uses time.Now.
	Now func() time.Time
}

// ExpireKeys removes every recorded key whose lifetime has elapsed, and forgets
// the record afterwards. It is best-effort by design — it runs on the way into
// a session that must open regardless — so a key that cannot be removed is
// logged and the rest are still tried.
func (e Expirer) ExpireKeys(ctx context.Context) error {
	added, err := e.Records.Added()
	if err != nil {
		return fmt.Errorf("read key records: %w", err)
	}
	now := e.now()
	for _, key := range added {
		if key.KeyFile == "" || key.ExpiresAt.IsZero() || now.Before(key.ExpiresAt) {
			continue
		}
		e.remove(ctx, key, now)
	}
	return nil
}

// remove runs ssh-add -d for one key whose time is up and disposes of its
// record.
//
// A refusal is not a reason to keep the record: `ssh-add -d` fails when the
// agent does not have the key, and when the file naming it is no longer there
// to name it with, and neither of those is going to be truer next session — so
// what is written down would go on asking for a removal that cannot happen. A
// failure to start ssh-add at all is different: nothing was asked of the agent,
// so the record stays and the next session tries again.
func (e Expirer) remove(ctx context.Context, key AddedKey, now time.Time) {
	name := filepath.Base(key.KeyFile)
	overdue := now.Sub(key.ExpiresAt).Round(time.Second)
	held := now.Sub(key.AddedAt).Round(time.Second)
	res, err := e.Runner.Run(ctx, run.Cmd{
		Name:    "ssh-add",
		Args:    []string{"-d", key.KeyFile},
		Env:     e.env(),
		Timeout: removeTimeout,
	})
	if err != nil {
		e.logf("ERROR", "could not run ssh-add to take out %s, whose lifetime ran out %s ago: %v",
			name, overdue, err)
		return
	}
	if res.Code != 0 {
		e.logf("INFO", "the agent does not have %s to take out (ssh-add exited %d); its lifetime ran out %s ago",
			name, res.Code, overdue)
	} else {
		e.logf("INFO", "took %s out of the agent after %s: its lifetime ran out %s ago", name, held, overdue)
	}
	if err := e.Records.Forget(key.KeyFile); err != nil {
		e.logf("ERROR", "could not forget the record for %s: %v", name, err)
	}
}

func (e Expirer) env() []string {
	if e.Endpoint == "" {
		return nil
	}
	return []string{"SSH_AUTH_SOCK=" + e.Endpoint}
}

func (e Expirer) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e Expirer) logf(level, format string, args ...any) {
	if e.Log == nil {
		return
	}
	_ = e.Log.Log(level, fmt.Sprintf(format, args...))
}
