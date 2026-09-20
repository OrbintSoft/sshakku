package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/move"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// moveKeysCmdName is what a user types to reach this.
const moveKeysCmdName = "move-keys"

// moveKeys puts the keys SSHakku loads into a directory the user names, gives
// that directory and the files in it what this system expects of a key, and
// writes the new location into the user's own configuration — so the same keys
// go on being loaded at every login with nothing else to change.
//
// Everything that can be refused is refused before the first file moves, and a
// failure part way through puts back what it already moved. A private key is in
// one place before and one place after: a copy left behind in a directory
// somebody has decided is the wrong one is the whole problem over again.
func (d deps) moveKeys(stdout, stderr io.Writer, args []string) int {
	dir, ok := parseMoveKeysArgs(stderr, args)
	if !ok {
		return 2
	}

	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir)
	log := sessionlog.New(layout.LogFile)
	enumerator := loadSettings(layout, moveKeysCmdName, log).KeyEnumerator(env.Home)
	keyFiles, err := enumerator.Keys()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: move-keys: %v\n", err)
		return 1
	}
	if len(keyFiles) == 0 {
		_, _ = fmt.Fprintf(stdout, "No keys to move in %s\n", enumerator.Dir)
		return 0
	}

	plan := move.PlanFor(dir, keyFiles, pathIsThere)
	if refusal := moveKeysRefusal(plan, enumerator.Dir, layout.ConfigDir); refusal != "" {
		_, _ = fmt.Fprintf(stderr, "sshakku: move-keys: %s\n", refusal)
		return 1
	}
	if prepared := d.prepareKeyDir(dir); prepared != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: move-keys: %v\n", prepared)
		return 1
	}

	res, err := move.Apply(plan, os.Rename, d.permitKey)
	if err != nil {
		moveKeysStopped(stderr, log, err, res)
		return 1
	}
	if err := config.SetKeyDir(layout.ConfigDir, dir); err != nil {
		// The keys are in the new directory and nothing says so, which is the
		// one state this command exists to avoid. Put them back.
		back, _ := move.Apply(move.Plan{Moves: reversed(plan.Moves)}, os.Rename, keepingWhateverTheyHad)
		moveKeysStopped(stderr, log, err, move.Result{Stranded: back.Stranded})
		return 1
	}

	for _, path := range res.Moved {
		_ = log.Log("INFO", "move-keys: moved "+path)
	}
	_, _ = fmt.Fprintf(stdout, "Moved %d files to %s\n", len(res.Moved), dir)
	_, _ = fmt.Fprintf(stdout, "%s now says that is where your keys are.\n", config.MainFile(layout.ConfigDir))
	return 0
}

// parseMoveKeysArgs reads the one argument this takes: the directory to move
// the keys into. The bool reports whether the arguments were usable.
func parseMoveKeysArgs(stderr io.Writer, args []string) (string, bool) {
	var named []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			_, _ = fmt.Fprintf(stderr, "sshakku: move-keys: unknown argument %q\n", arg)
			return "", false
		}
		named = append(named, arg)
	}
	if len(named) != 1 {
		_, _ = fmt.Fprintln(stderr, "sshakku: move-keys: name one directory to move the keys into")
		return "", false
	}
	// Made absolute against the directory the user is standing in, which is
	// what somebody typing a path at a terminal means by a relative one. It is
	// also what goes into the configuration, where a relative path would be
	// read against the home directory instead and name somewhere else.
	dir, err := filepath.Abs(named[0])
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: move-keys: %v\n", err)
		return "", false
	}
	return dir, true
}

// moveKeysRefusal is everything that stops a move before anything moves, as the
// sentence the user is owed, or "" where nothing does.
//
// They are all checked here, ahead of every change, because the promise is that
// nothing is done by halves: a refusal discovered after two keys have moved is
// a user with keys in two places.
func moveKeysRefusal(plan move.Plan, from, configDir string) string {
	if sameDir(plan.Dir, from) {
		return "your keys are already in " + plan.Dir
	}
	if elsewhere := config.KeyDirDecidedElsewhere(
		config.LoadSources(configDir), os.LookupEnv, configDir,
	); elsewhere != "" {
		return fmt.Sprintf(
			"%s decides where your keys are, and it is read after %s — moving them would leave "+
				"every login looking in the old place. Change it there instead, or take key_dir out of it",
			elsewhere, config.MainFile(configDir))
	}
	if taken := plan.Blocked(pathIsThere); len(taken) > 0 {
		return fmt.Sprintf("these are already in %s, and nothing here overwrites a key: %s",
			plan.Dir, strings.Join(taken, ", "))
	}
	if err := config.WritableConfig(configDir); err != nil {
		return err.Error()
	}
	return ""
}

// prepareKeyDir makes the directory the keys are going into and gives it what
// this system expects, so that a key generated there later is as private as the
// ones moved there now.
//
// A directory left behind by a run that then refused is an empty directory, and
// that is the one change this command may leave: the promise is about the keys.
func (d deps) prepareKeyDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := d.permitKey(dir, move.Directory); err != nil {
		return fmt.Errorf("set the permissions of %s: %w", dir, err)
	}
	return nil
}

// moveKeysStopped says what stopped a run and, where it applies, which files a
// person has to deal with themselves.
func moveKeysStopped(stderr io.Writer, log *sessionlog.Logger, err error, res move.Result) {
	_, _ = fmt.Fprintf(stderr, "sshakku: move-keys: %v\n", err)
	_ = log.Log("ERROR", "move-keys: "+err.Error())
	if len(res.Stranded) == 0 {
		_, _ = fmt.Fprintln(stderr, "Nothing was moved; your keys are where they were.")
		return
	}
	_, _ = fmt.Fprintln(stderr,
		"These could not be put back where they came from, and are still in the new directory:")
	for _, path := range res.Stranded {
		_, _ = fmt.Fprintf(stderr, "  %s\n", path)
		_ = log.Log("ERROR", "move-keys: stranded "+path)
	}
}

// reversed turns a plan's moves round, which is how the keys go back when
// everything moved and the configuration would not be written.
func reversed(moves []move.Move) []move.Move {
	back := make([]move.Move, 0, len(moves))
	for _, m := range moves {
		back = append(back, move.Move{From: m.To, To: m.From, Kind: m.Kind})
	}
	return back
}

// pathIsThere answers whether a path exists at all, symbolic links included: a
// link is still a name that is taken.
func pathIsThere(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// sameDir reports whether two paths name one directory.
//
// It compares the cleaned paths and nothing cleverer. A system that does not
// distinguish .ssh from .SSH would let that pair through here, and the check
// for a destination already taken catches it a moment later: on such a system
// every key's destination is the key itself, which is a name already in use.
func sameDir(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// keepingWhateverTheyHad is the permitting step of a run that is only putting
// files back. They are returning to a directory that already gave them what
// they needed, and changing anything there is not this run's business.
func keepingWhateverTheyHad(string, move.Kind) error { return nil }
