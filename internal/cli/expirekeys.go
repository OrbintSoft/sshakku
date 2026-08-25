package cli

import (
	"context"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// expireKeys takes out of the agent every key sshakku added whose lifetime has
// elapsed, on a system whose agent expires nothing itself. Every session does
// it on the way in — not only the ones somebody is sitting at, since a key past
// its time is past it whoever opens the next shell, and a machine that mostly
// runs work handed to it would otherwise keep the key for as long as nobody
// logs in.
//
// It costs a session that has nothing to expire one directory read: ssh-add is
// run only for a key whose time has actually come.
//
// What goes wrong here is written to the session log and nowhere else. This
// runs while a shell is opening, and the stream a person would read it on is
// the one that shell is being handed — a store that cannot be read is not
// something to act on at that moment, and the session opens either way.
func (d deps) expireKeys(ctx context.Context, layout paths.Layout, live agent.Endpoint) {
	if !expiryIsThisSessionsJob(d.agentKeepsLifetimes, live.Native()) {
		return
	}
	log := sessionlog.New(layout.LogFile)
	expirer := keys.Expirer{
		Records:  keyRecords{store: keystate.Store{Dir: keystateDir(layout)}},
		Runner:   d.runner,
		Endpoint: live.Native(),
		Log:      log,
	}
	if err := expirer.ExpireKeys(ctx); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("shell-init: expire keys: %v", err))
	}
}

// expiryIsThisSessionsJob reports whether this session is the one that has to
// take expired keys out of the agent.
//
// Where the agent holds lifetimes it has already dropped the key at its own
// deadline, and a second opinion here could only remove what somebody re-added
// since. Where there is no live endpoint there is no agent to take anything out
// of. Which system this is comes in as the answer it is, so both outcomes stay
// checkable from either machine.
func expiryIsThisSessionsJob(agentKeepsLifetimes bool, endpoint string) bool {
	return !agentKeepsLifetimes && endpoint != ""
}

// keyRecords adapts what sshakku wrote down as it added keys to what an
// Expirer reads: when each key was added and when it stops being one this
// session may keep.
type keyRecords struct{ store keystate.Store }

// Added lists the keys recorded as added. A record whose lifetime is zero
// carries no expiry time, which is what says the key never runs out.
func (k keyRecords) Added() ([]keys.AddedKey, error) {
	recs, err := k.store.Records()
	if err != nil {
		return nil, err
	}
	added := make([]keys.AddedKey, 0, len(recs))
	for _, rec := range recs {
		key := keys.AddedKey{KeyFile: rec.KeyFile, AddedAt: rec.AddedAt}
		if expiresAt, hasExpiry := rec.ExpiresAt(); hasExpiry {
			key.ExpiresAt = expiresAt
		}
		added = append(added, key)
	}
	return added, nil
}

// Forget drops the record for the key in keyfile; the store names its records
// after the file's base name.
func (k keyRecords) Forget(keyfile string) error { return k.store.Clear(keyfile) }
