package keepassxc

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// failingReader stands in for an entropy source that cannot deliver. The real
// one does not fail, which is exactly why the branches that handle it would
// otherwise never run.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// withoutEntropy points the package's randomness at a reader that always fails,
// and restores it when the test ends.
func withoutEntropy(t *testing.T) {
	t.Helper()
	swapEntropy(t, failingReader{})
}

// exhaustibleReader delivers real randomness a fixed number of times and then
// fails. Connect draws several times, so failing on the first draw would stop
// it before the later ones ever run.
type exhaustibleReader struct {
	left int
}

func (r *exhaustibleReader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, errors.New("no entropy left")
	}
	r.left--
	for i := range p {
		p[i] = byte(i + 1)
	}
	return len(p), nil
}

// swapEntropy installs src as the package's randomness for the test.
func swapEntropy(t *testing.T, src io.Reader) {
	t.Helper()
	original := randReader
	randReader = src
	t.Cleanup(func() { randReader = original })
}

func TestConnectReportsAFailureGeneratingItsClientId(t *testing.T) {
	s := newFakeServer(t)
	// One draw succeeds — the session key pair — and the client id's fails.
	swapEntropy(t, &exhaustibleReader{left: 1})
	if _, err := Connect(s.dial(), time.Second); err == nil {
		t.Fatal("Connect must fail when it cannot generate its client id")
	}
}

func TestConnectReportsAFailureGeneratingTheExchangeNonce(t *testing.T) {
	s := newFakeServer(t)
	// The key pair and the client id succeed; the key exchange's nonce fails.
	swapEntropy(t, &exhaustibleReader{left: 2})
	if _, err := Connect(s.dial(), time.Second); err == nil {
		t.Fatal("Connect must fail when it cannot generate the exchange nonce")
	}
}

func TestARequestThatCannotGenerateANonceIsRefused(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	withoutEntropy(t)
	if _, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"}); err == nil {
		t.Fatal("a request with no nonce must not be sent — the nonce is what ties the reply to it")
	}
}

func TestARequestThatCannotBeEncodedIsRefused(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	var out map[string]any
	// A channel has no JSON form, so sealing it cannot succeed.
	if err := c.request(actionGetLogins, make(chan int), &out); err == nil {
		t.Fatal("a request that cannot be encoded must be reported")
	}
}

func TestAServerThatHangsUpMidRequestIsReported(t *testing.T) {
	s := newFakeServer(t)
	s.hangUpOn = actionGetLogins
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	if err == nil {
		t.Fatal("a peer that goes away after accepting the request must fail the call, not hang")
	}
	if !strings.Contains(err.Error(), "reading the reply") {
		t.Errorf("error = %v, want it to say the reply could not be read", err)
	}
}

func TestNewNonceReportsAnEntropyFailure(t *testing.T) {
	withoutEntropy(t)
	if _, err := newNonce(); err == nil {
		t.Fatal("a nonce that could not be generated must be reported, not returned as zeroes")
	}
}

func TestNewKeyPairReportsAnEntropyFailure(t *testing.T) {
	withoutEntropy(t)
	if _, err := newKeyPair(); err == nil {
		t.Fatal("a key pair that could not be generated must be reported")
	}
}

func TestConnectReportsAnEntropyFailure(t *testing.T) {
	s := newFakeServer(t)
	withoutEntropy(t)
	if _, err := Connect(s.dial(), time.Second); err == nil {
		t.Fatal("Connect must fail when it cannot generate its session key")
	}
}

func TestAssociateReportsAnEntropyFailure(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	withoutEntropy(t)
	if _, err := c.Associate(); err == nil {
		t.Fatal("Associate must fail when it cannot generate its identification key")
	}
}

func TestSetLoginReportsAnEntropyFailure(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	withoutEntropy(t)
	err := c.SetLogin("ssh://k", "k", "p", "", "", Association{ID: "db", IDKey: "key"})
	if err == nil {
		t.Fatal("SetLogin must fail when it cannot generate its nonce")
	}
}

func TestCloseReleasesTheConnection(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// A second request must not succeed on a released connection.
	if _, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"}); err == nil {
		t.Error("a request after Close must fail")
	}
}

func TestDecodeKeyRejectsWhatIsNotBase64(t *testing.T) {
	if _, err := decodeKey("not base64!!"); err == nil {
		t.Fatal("a key that is not base64 must be refused")
	}
}

func TestSealReportsAPayloadItCannotEncode(t *testing.T) {
	keys, err := newKeyPair()
	if err != nil {
		t.Fatalf("newKeyPair: %v", err)
	}
	var nonce [nonceLen]byte
	// A channel has no JSON form, so this is a payload that cannot be encoded.
	if _, err := seal(make(chan int), nonce, keys.public, keys.secret); err == nil {
		t.Fatal("a payload that cannot be encoded must be reported")
	}
}

func TestOpenRejectsWhatIsNotBase64(t *testing.T) {
	keys, err := newKeyPair()
	if err != nil {
		t.Fatalf("newKeyPair: %v", err)
	}
	var nonce [nonceLen]byte
	var out map[string]any
	if err := open("not base64!!", nonce, keys.public, keys.secret, &out); err == nil {
		t.Fatal("a message that is not base64 must be refused")
	}
}

func TestOpenRejectsAPayloadOfTheWrongShape(t *testing.T) {
	keys, err := newKeyPair()
	if err != nil {
		t.Fatalf("newKeyPair: %v", err)
	}
	var nonce [nonceLen]byte
	sealed, err := seal("a bare string", nonce, keys.public, keys.secret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	var out map[string]any
	if err := open(sealed, nonce, keys.public, keys.secret, &out); err == nil {
		t.Fatal("a payload that decrypts but does not fit must be reported")
	}
}

func TestKeyExchangeWithNoKeyIsAnError(t *testing.T) {
	s := newFakeServer(t)
	s.prelude[actionChangePublicKeys] = []envelope{{
		Action:  actionChangePublicKeys,
		Success: "true",
	}}
	_, err := Connect(s.dial(), time.Second)
	if err == nil {
		t.Fatal("a key exchange that returned no key must not be treated as done")
	}
	if !strings.Contains(err.Error(), "no public key") {
		t.Errorf("error = %v, want it to say the key was missing", err)
	}
}

func TestReplyWithNoMessageIsAnError(t *testing.T) {
	s := newFakeServer(t)
	s.prelude[actionGetLogins] = []envelope{{
		Action:  actionGetLogins,
		Success: "true",
	}}
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	if err == nil {
		t.Fatal("a reply carrying nothing to decrypt must be an error")
	}
	if !strings.Contains(err.Error(), "no message") {
		t.Errorf("error = %v, want it to say the message was missing", err)
	}
}

func TestNamedFailureIsReportedAsGiven(t *testing.T) {
	s := newFakeServer(t)
	s.failWith[actionGetLogins] = envelope{Success: "false", Error: "something specific"}
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	if err == nil || !strings.Contains(err.Error(), "something specific") {
		t.Fatalf("err = %v, want KeePassXC's own words passed through", err)
	}
}

func TestUnnamedFailureStillReportsTheAction(t *testing.T) {
	s := newFakeServer(t)
	s.failWith[actionGetLogins] = envelope{Success: "false"}
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	if err == nil || !strings.Contains(err.Error(), actionGetLogins) {
		t.Fatalf("err = %v, want the failing action named", err)
	}
}

// brokenConn fails the transport operation the test is interested in. It is a
// transport, upstream of everything the client decides, so replacing it does
// not stub any behaviour under test.
type brokenConn struct {
	net.Conn
	failDeadline bool
	failWrite    bool
}

func (c *brokenConn) SetDeadline(t time.Time) error {
	if c.failDeadline {
		return errors.New("cannot set a deadline")
	}
	return c.Conn.SetDeadline(t)
}

func (c *brokenConn) Write(p []byte) (int, error) {
	if c.failWrite {
		return 0, errors.New("the socket went away")
	}
	return c.Conn.Write(p)
}

func TestADeadlineThatCannotBeSetIsReported(t *testing.T) {
	s := newFakeServer(t)
	conn := &brokenConn{Conn: s.dial(), failDeadline: true}
	if _, err := Connect(conn, time.Second); err == nil {
		t.Fatal("a deadline that cannot be set must not be ignored — it is what bounds the wait")
	}
}

func TestAWriteThatFailsIsReported(t *testing.T) {
	s := newFakeServer(t)
	conn := &brokenConn{Conn: s.dial(), failWrite: true}
	if _, err := Connect(conn, time.Second); err == nil {
		t.Fatal("a request that could not be sent must be reported")
	}
}

func TestAServerThatHangsUpIsReported(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	// Closing the listener's connection from our side is the closest a test
	// gets to KeePassXC exiting mid-session.
	if err := c.conn.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if _, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"}); err == nil {
		t.Fatal("a session whose peer went away must fail, not wait")
	}
}

func TestConnectDefaultsItsTimeout(t *testing.T) {
	s := newFakeServer(t)
	c, err := Connect(s.dial(), 0)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want DefaultTimeout for a caller that named none", c.timeout)
	}
}

func TestSocketPaths(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		tempDir    string
		runtimeDir string
		want       []string
	}{
		{
			name:       "macOS looks in the per-user temporary directory",
			goos:       "darwin",
			tempDir:    "/var/folders/ab/T",
			runtimeDir: "/run/user/1000",
			want: []string{
				"/var/folders/ab/T/" + socketName,
				"/tmp/" + socketName,
			},
		},
		{
			name:       "Linux prefers the per-application runtime directory",
			goos:       "linux",
			tempDir:    "/tmp",
			runtimeDir: "/run/user/1000",
			want: []string{
				"/run/user/1000/app/org.keepassxc.KeePassXC/" + socketName,
				"/run/user/1000/" + socketName,
				"/tmp/" + socketName,
			},
		},
		{
			name:       "Linux with no runtime directory has only the fallback",
			goos:       "linux",
			tempDir:    "/tmp",
			runtimeDir: "",
			want:       []string{"/tmp/" + socketName},
		},
		{
			name:       "macOS whose temporary directory is /tmp does not repeat it",
			goos:       "darwin",
			tempDir:    "/tmp",
			runtimeDir: "",
			want:       []string{"/tmp/" + socketName},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := socketPaths(tc.goos, tc.tempDir, tc.runtimeDir)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("path %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestSocketPathsUsesTheRunningPlatform proves the exported wrapper actually
// consults the environment rather than returning a fixed list.
func TestSocketPathsUsesTheRunningPlatform(t *testing.T) {
	paths := SocketPaths()
	if len(paths) == 0 {
		t.Fatal("SocketPaths must always offer at least the fallback")
	}
	last := paths[len(paths)-1]
	if last != "/tmp/"+socketName {
		t.Errorf("the last candidate is %q, want the /tmp fallback", last)
	}
}
