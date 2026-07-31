package keys

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// keepassxcCLIBin is KeePassXC's command-line interface. It works on the
// database file rather than on a running KeePassXC, so it needs the database's
// own password every time it is invoked — which is why it is the fallback and
// not the way SSHakku reaches KeePassXC by default.
const keepassxcCLIBin = "keepassxc-cli"

// keepassxcCLIGroup is the group entries are kept in, matching the one the
// local-protocol route creates, so the same database read either way looks the
// same to the user.
const keepassxcCLIGroup = "SSHakku"

// ErrNoDatabase reports that no database file was configured. The CLI route
// works on a file and cannot discover which one: a KeePassXC that is running
// knows what it has open, a file on disk does not announce itself.
var ErrNoDatabase = errors.New("keepassxc: no database file is configured")

// ErrPasswordNotAccepted reports that keepassxc-cli would not take the database
// password on its standard input.
//
// Feeding it there is not a documented interface — keepassxc-cli has no
// "--password-stdin" and reads its prompt from whatever it is given — so
// SSHakku treats it as something to find out rather than to rely on. When it
// stops working, that is what this says, instead of an unexplained failure or a
// prompt no one can answer.
var ErrPasswordNotAccepted = errors.New("keepassxc: keepassxc-cli would not take the database password on standard input")

// KeePassXCCLIBackend reaches a KeePassXC database through keepassxc-cli,
// without a running KeePassXC.
//
// It cannot be silent: opening the database needs its password, and that is
// asked for. This is the route's defining difference from the local protocol,
// which talks to a database the user has already unlocked.
//
// The password travels on the child's standard input, never in argv, so it
// cannot be read out of the process table.
type KeePassXCCLIBackend struct {
	Runner   Runner
	Prompter Prompter
	// Database is the .kdbx file to open.
	Database string
	// KeyFile is an optional key file the database also requires.
	KeyFile string
	// Timeout bounds each call. The CLI waits on a person, so this is the
	// interactive budget rather than the shorter command one.
	Timeout time.Duration

	// password is the database password for this process's lifetime, asked for
	// once rather than once per operation. It is never written anywhere.
	password string
}

// entryPath is where a key's entry lives inside the database.
func keepassxcCLIEntryPath(service string) string {
	return keepassxcCLIGroup + "/" + service
}

// unlock returns the database password, asking for it the first time.
func (b *KeePassXCCLIBackend) unlock() (string, error) {
	if b.password != "" {
		return b.password, nil
	}
	if b.Database == "" {
		return "", ErrNoDatabase
	}
	password, err := b.Prompter.Prompt("your KeePassXC database password")
	if err != nil {
		return "", err
	}
	b.password = password
	return password, nil
}

// run invokes keepassxc-cli with the database password — and anything else the
// subcommand will ask for — on standard input, in the order it asks.
func (b *KeePassXCCLIBackend) run(args []string, extraInput ...string) (Result, error) {
	password, err := b.unlock()
	if err != nil {
		return Result{}, err
	}
	full := append([]string{args[0], "-q"}, args[1:]...)
	if b.KeyFile != "" {
		full = append(full, "--key-file", b.KeyFile)
	}
	input := password + "\n"
	for _, extra := range extraInput {
		input += extra + "\n"
	}
	res, err := b.Runner.Run(Cmd{
		Name:    keepassxcCLIBin,
		Args:    full,
		Stdin:   input,
		Timeout: b.Timeout,
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// refusedPassword reports whether a failure looks like keepassxc-cli declining
// the password on standard input rather than the password being wrong. The two
// deserve different words: one is a mistyped password, the other is SSHakku
// relying on an interface that changed under it.
func refusedPassword(res Result) bool {
	text := strings.ToLower(string(res.Stderr))
	return strings.Contains(text, "failed to open") && strings.Contains(text, "terminal")
}

// Lookup returns the passphrase stored for service.
func (b *KeePassXCCLIBackend) Lookup(service string) (string, bool, error) {
	res, err := b.run([]string{"show", "-s", "-a", "Password", b.Database, keepassxcCLIEntryPath(service)})
	if err != nil {
		return "", false, err
	}
	if res.Code != 0 {
		if refusedPassword(res) {
			return "", false, ErrPasswordNotAccepted
		}
		// Anything else — including no such entry — is a miss, so the loader
		// asks and stores rather than failing the shell.
		return "", false, nil
	}
	return strings.TrimRight(string(res.Stdout), "\n"), true, nil
}

// Store saves passphrase for service, editing the entry when it already exists
// so a re-stored passphrase does not leave a second copy in the database.
func (b *KeePassXCCLIBackend) Store(service, label, passphrase string) error {
	// label has nowhere to go: the entry is named by its path, which is built
	// from the service identifier.
	_ = label
	path := keepassxcCLIEntryPath(service)

	verb := "add"
	if _, found, err := b.Lookup(service); err != nil {
		return err
	} else if found {
		verb = "edit"
	}

	if verb == "add" {
		// keepassxc-cli will not create an entry inside a group that is not
		// there, and a fresh database has no SSHakku group. Creating it is
		// idempotent in effect: an "it already exists" failure is not
		// distinguished, because the add that follows is the real test of
		// whether the group is usable.
		if _, err := b.run([]string{"mkdir", b.Database, keepassxcCLIGroup}); err != nil {
			return err
		}
	}

	// -p makes keepassxc-cli ask for the entry's own password, which follows
	// the database password on standard input.
	res, err := b.run([]string{verb, "-p", b.Database, path}, passphrase)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		if refusedPassword(res) {
			return ErrPasswordNotAccepted
		}
		return fmt.Errorf("keepassxc-cli %s %s: %s", verb, path, firstLine(res.Stderr))
	}
	return nil
}

// Delete removes the entry for service. Unlike the local-protocol route, the
// CLI can delete, so `sshakku forget` works here.
func (b *KeePassXCCLIBackend) Delete(service string) error {
	res, err := b.run([]string{"rm", b.Database, keepassxcCLIEntryPath(service)})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		if refusedPassword(res) {
			return ErrPasswordNotAccepted
		}
		// A missing entry is success: forgetting an already-forgotten key is
		// not a failure.
		return nil
	}
	return nil
}

// List returns every entry SSHakku keeps in the database.
func (b *KeePassXCCLIBackend) List() ([]string, error) {
	res, err := b.run([]string{"ls", "-f", b.Database, keepassxcCLIGroup})
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		if refusedPassword(res) {
			return nil, ErrPasswordNotAccepted
		}
		// No group yet means nothing has been stored.
		return nil, nil
	}
	var services []string
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		name := strings.TrimSpace(line)
		// A trailing slash marks a nested group, not an entry.
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		services = append(services, strings.TrimPrefix(name, keepassxcCLIGroup+"/"))
	}
	return services, nil
}

// firstLine is the first line of output, for an error message that should stay
// one line.
func firstLine(out []byte) string {
	text := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
}
