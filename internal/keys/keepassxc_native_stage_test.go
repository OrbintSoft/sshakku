//go:build unix

package keys

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
	"github.com/stretchr/testify/require"
)

// Staging a KeePassXC for the full round to talk to, where nothing else
// provides one. It is scaffolding, not the subject: everything the round
// asserts still happens through the real binary against the app started here.
//
// Two things a person normally does have to happen without one, and each has
// exactly one way that works:
//
//   - the database is opened with --pw-stdin, and the stream is then held open.
//     Closing it after writing the password leaves the database locked, which is
//     what an earlier attempt died of; on Linux's build of the same version the
//     option crashes the app outright, so this path is macOS's alone.
//
//   - the association is written into the database instead of being approved in
//     a dialog. KeePassXC keeps only a name and the client's identification
//     public key for it, under Meta/CustomData, so there is nothing secret to
//     forge — and the alternative is a dialog that waits forever on a machine
//     where nobody can click it.
//
// The approval is a precondition of the route, not the promise under test: the
// round still has to ask for a passphrase, save it, and give it back silently on
// its own.

// browserSettings is the settings fragment that turns the local protocol on,
// shared with the container session so both are configured from one file.
const browserSettings = "../../test/containers/keepassxc-browser.ini"

// stagedPassword unlocks the database this test makes and nothing else.
const stagedPassword = "sshakku-native-full-round-database"

// stageKeePassXC starts a KeePassXC of this test's own on a database made for
// it, and returns once the app is answering for an open database.
func stageKeePassXC(t *testing.T, app, root, stateDir string) {
	t.Helper()

	_, err := exec.LookPath("keepassxc-cli")
	require.NoError(t, err, "keepassxc-cli makes the database the app is to open")

	settings := filepath.Join(root, "keepassxc.ini")
	fragment, err := os.ReadFile(browserSettings)
	require.NoError(t, err, "the settings fragment that turns the local protocol on")
	require.NoError(t, os.WriteFile(settings, fragment, 0o600), "write the settings for the staged app")

	database := stageDatabase(t, root, stateDir)

	// --config takes the settings file to use, so nothing here depends on where
	// this build would otherwise keep them, and nothing writes to the settings
	// of whoever is running the test.
	cmd := exec.CommandContext(t.Context(), app, "--config", settings, "--pw-stdin", database)
	// Whatever the app says is kept, because the interesting failure is the one
	// where it starts, answers, and never opens the database: without its own
	// account of that there is nothing to diagnose but a timeout.
	said := &lockedBuffer{}
	cmd.Stdout, cmd.Stderr = said, said
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err, "the password reaches the app on its standard input")
	require.NoErrorf(t, cmd.Start(), "start KeePassXC at %s", app)
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	_, err = io.WriteString(stdin, stagedPassword+"\n")
	require.NoError(t, err, "hand the password to the staged app")
	// stdin stays open on purpose; see above.

	waitForOpenDatabase(t, cmd, said)
}

// lockedBuffer collects what the app writes while the test reads it from
// another goroutine, which is what os/exec's copying amounts to here.
type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// stageDatabase makes a database with the association already in it, and writes
// the matching half where SSHakku looks for it.
func stageDatabase(t *testing.T, root, stateDir string) string {
	t.Helper()

	plain := filepath.Join(root, "plain.kdbx")
	keepassxcCLI(t, stagedPassword+"\n"+stagedPassword+"\n", "db-create", "-p", plain)

	exported := keepassxcCLI(t, stagedPassword+"\n", "export", "-f", "xml", plain)

	var key [32]byte
	_, err := rand.Read(key[:])
	require.NoError(t, err, "make an identification key")
	const name = "sshakku-full-round"
	idKey := base64.StdEncoding.EncodeToString(key[:])

	// Meta's CustomData is the first in the document; the groups and entries that
	// carry their own come after it.
	const anchor = "<CustomData>"
	at := strings.Index(exported, anchor)
	require.GreaterOrEqual(t, at, 0, "the exported database has no CustomData to put an association in")
	at += len(anchor)
	item := fmt.Sprintf("<Item><Key>KPXC_BROWSER_%s</Key><Value>%s</Value></Item>", name, idKey)
	withAssociation := filepath.Join(root, "seeded.xml")
	require.NoError(t, os.WriteFile(withAssociation, []byte(exported[:at]+item+exported[at:]), 0o600),
		"write the database with the association in it")

	database := filepath.Join(root, "wallet.kdbx")
	keepassxcCLI(t, stagedPassword+"\n"+stagedPassword+"\n", "import", "-p", withAssociation, database)

	require.NoError(t, os.MkdirAll(stateDir, 0o700), "make the state dir")
	stored, err := json.Marshal(map[string]any{"version": 1, "id": name, "idKey": idKey})
	require.NoError(t, err, "encode the association")
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "keepassxc-association.json"), stored, 0o600),
		"write the association SSHakku will load")
	return database
}

// keepassxcCLI runs keepassxc-cli with what it would otherwise prompt a person
// for, and returns its output.
func keepassxcCLI(t *testing.T, stdin string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "keepassxc-cli", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoErrorf(t, err, "keepassxc-cli %v: %s", args, stderr.String())
	return string(out)
}

// waitForOpenDatabase blocks until KeePassXC answers for a database that is
// actually open. The socket is not that signal: KeePassXC listens on it from
// dialAnyKeePassXCSocket connects to the first of KeePassXC's candidate sockets
// that answers. The staging helper only needs to know whether something is
// listening yet, so it dials for itself rather than reaching into the wallet
// for a lookup that reports which paths it tried.
func dialAnyKeePassXCSocket(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer
	var err error
	for _, path := range keepassxc.SocketPaths() {
		var conn net.Conn
		if conn, err = dialer.DialContext(ctx, "unix", path); err == nil {
			return conn, nil
		}
	}
	if err == nil {
		err = errors.New("keepassxc advertises no socket path on this system")
	}
	return nil, err
}

// startup and answers "database not opened" until one is, so a test that waited
// on the socket alone would go on to fail for a reason it had already been told.
func waitForOpenDatabase(t *testing.T, app *exec.Cmd, said *lockedBuffer) {
	t.Helper()

	deadline := time.Now().Add(45 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		conn, err := dialAnyKeePassXCSocket(t.Context())
		if err == nil {
			client, connErr := keepassxc.Connect(conn, 5*time.Second, 5*time.Second)
			if connErr == nil {
				_, last = client.GetLogins("sshakku://staging-readiness", keepassxc.Association{})
				_ = client.Close()
				// Anything but a locked database means one is open. What it
				// answers about an unknown URL or an empty association is not
				// this function's business.
				if last == nil || !strings.Contains(last.Error(), "locked") {
					return
				}
			} else {
				last = connErr
				_ = conn.Close()
			}
		} else {
			last = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Which of the two it is decides what to do about it: an app that is gone
	// crashed on the way in, while one still running is holding the database
	// shut for a reason it may have printed.
	// Signal 0 asks the kernel rather than os/exec: ProcessState stays empty
	// until the process is waited for, and this one is only waited for on the
	// way out, so it would report every corpse as healthy.
	alive := "still running"
	if err := app.Process.Signal(syscall.Signal(0)); err != nil {
		alive = "no longer running (" + err.Error() + ")"
	}
	require.FailNowf(t, "KeePassXC never reported an open database",
		"last answer: %v\napp %s, and said:\n%s", last, alive, said.String())
}
