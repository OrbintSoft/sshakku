//go:build unix

package keys

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
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

	if _, err := exec.LookPath("keepassxc-cli"); err != nil {
		t.Fatal("keepassxc-cli is not on PATH, so no database can be made for the app to open")
	}

	settings := filepath.Join(root, "keepassxc.ini")
	fragment, err := os.ReadFile(browserSettings)
	if err != nil {
		t.Fatalf("read the browser settings: %v", err)
	}
	if err := os.WriteFile(settings, fragment, 0o600); err != nil {
		t.Fatalf("write the settings for the staged app: %v", err)
	}

	database := stageDatabase(t, root, stateDir)

	// --config takes the settings file to use, so nothing here depends on where
	// this build would otherwise keep them, and nothing writes to the settings
	// of whoever is running the test.
	cmd := exec.Command(app, "--config", settings, "--pw-stdin", database)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("open the staged app's stdin: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start KeePassXC at %s: %v", app, err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	if _, err := io.WriteString(stdin, stagedPassword+"\n"); err != nil {
		t.Fatalf("hand the password to the staged app: %v", err)
	}
	// stdin stays open on purpose; see above.

	waitForOpenDatabase(t)
}

// stageDatabase makes a database with the association already in it, and writes
// the matching half where SSHakku looks for it.
func stageDatabase(t *testing.T, root, stateDir string) string {
	t.Helper()

	plain := filepath.Join(root, "plain.kdbx")
	keepassxcCLI(t, stagedPassword+"\n"+stagedPassword+"\n", "db-create", "-p", plain)

	exported := keepassxcCLI(t, stagedPassword+"\n", "export", "-f", "xml", plain)

	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("make an identification key: %v", err)
	}
	const name = "sshakku-full-round"
	idKey := base64.StdEncoding.EncodeToString(key[:])

	// Meta's CustomData is the first in the document; the groups and entries that
	// carry their own come after it.
	const anchor = "<CustomData>"
	at := strings.Index(exported, anchor)
	if at < 0 {
		t.Fatal("the exported database has no CustomData to put an association in")
	}
	at += len(anchor)
	item := fmt.Sprintf("<Item><Key>KPXC_BROWSER_%s</Key><Value>%s</Value></Item>", name, idKey)
	withAssociation := filepath.Join(root, "seeded.xml")
	if err := os.WriteFile(withAssociation, []byte(exported[:at]+item+exported[at:]), 0o600); err != nil {
		t.Fatalf("write the database with the association in it: %v", err)
	}

	database := filepath.Join(root, "wallet.kdbx")
	keepassxcCLI(t, stagedPassword+"\n"+stagedPassword+"\n", "import", "-p", withAssociation, database)

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("make the state dir: %v", err)
	}
	stored, err := json.Marshal(map[string]any{"version": 1, "id": name, "idKey": idKey})
	if err != nil {
		t.Fatalf("encode the association: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "keepassxc-association.json"), stored, 0o600); err != nil {
		t.Fatalf("write the association SSHakku will load: %v", err)
	}
	return database
}

// keepassxcCLI runs keepassxc-cli with what it would otherwise prompt a person
// for, and returns its output.
func keepassxcCLI(t *testing.T, stdin string, args ...string) string {
	t.Helper()

	cmd := exec.Command("keepassxc-cli", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("keepassxc-cli %v: %v: %s", args, err, stderr.String())
	}
	return string(out)
}

// waitForOpenDatabase blocks until KeePassXC answers for a database that is
// actually open. The socket is not that signal: KeePassXC listens on it from
// startup and answers "database not opened" until one is, so a test that waited
// on the socket alone would go on to fail for a reason it had already been told.
func waitForOpenDatabase(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(45 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		conn, err := dialKeePassXCAt(keepassxc.SocketPaths())
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
	t.Fatalf("KeePassXC never reported an open database; last answer: %v", last)
}
