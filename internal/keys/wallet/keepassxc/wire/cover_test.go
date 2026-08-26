package wire

import (
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	_, err := Connect(s.dial(), time.Second, time.Second)
	assert.Error(t, err, "Connect must fail when it cannot generate its client id")
}

func TestConnectReportsAFailureGeneratingTheExchangeNonce(t *testing.T) {
	s := newFakeServer(t)
	// The key pair and the client id succeed; the key exchange's nonce fails.
	swapEntropy(t, &exhaustibleReader{left: 2})
	_, err := Connect(s.dial(), time.Second, time.Second)
	assert.Error(t, err, "Connect must fail when it cannot generate the exchange nonce")
}

func TestARequestThatCannotGenerateANonceIsRefused(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	withoutEntropy(t)
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	assert.Error(t, err, "a request with no nonce must not be sent — the nonce is what ties the reply to it")
}

func TestARequestThatCannotBeEncodedIsRefused(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	// A channel has no JSON form, so sealing it cannot succeed.
	err := c.request(actionGetLogins, make(chan int), &getLoginsReply{})
	assert.Error(t, err, "a request that cannot be encoded must be reported")
}

func TestAServerThatHangsUpMidRequestIsReported(t *testing.T) {
	s := newFakeServer(t)
	s.hangUpOn = actionGetLogins
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	require.Error(t, err, "a peer that goes away after accepting the request must fail the call, not hang")
	assert.ErrorContains(t, err, "reading the reply", "the error must say the reply could not be read")
}

func TestNewNonceReportsAnEntropyFailure(t *testing.T) {
	withoutEntropy(t)
	_, err := newNonce()
	assert.Error(t, err, "a nonce that could not be generated must be reported, not returned as zeroes")
}

func TestNewKeyPairReportsAnEntropyFailure(t *testing.T) {
	withoutEntropy(t)
	_, err := newKeyPair()
	assert.Error(t, err, "a key pair that could not be generated must be reported")
}

func TestConnectReportsAnEntropyFailure(t *testing.T) {
	s := newFakeServer(t)
	withoutEntropy(t)
	_, err := Connect(s.dial(), time.Second, time.Second)
	assert.Error(t, err, "Connect must fail when it cannot generate its session key")
}

func TestAssociateReportsAnEntropyFailure(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	withoutEntropy(t)
	_, err := c.Associate()
	assert.Error(t, err, "Associate must fail when it cannot generate its identification key")
}

func TestSetLoginReportsAnEntropyFailure(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	withoutEntropy(t)
	err := c.SetLogin("ssh://k", "k", "p", "", "", Association{ID: "db", IDKey: "key"})
	assert.Error(t, err, "SetLogin must fail when it cannot generate its nonce")
}

func TestCloseReleasesTheConnection(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	require.NoError(t, c.Close(), "Close")
	// A second request must not succeed on a released connection.
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	assert.Error(t, err, "a request after Close must fail")
}

func TestDecodeKeyRejectsWhatIsNotBase64(t *testing.T) {
	_, err := decodeKey("not base64!!")
	assert.Error(t, err, "a key that is not base64 must be refused")
}

func TestSealReportsAPayloadItCannotEncode(t *testing.T) {
	keys, err := newKeyPair()
	require.NoError(t, err, "newKeyPair")
	var nonce [nonceLen]byte
	// A channel has no JSON form, so this is a payload that cannot be encoded.
	_, err = seal(make(chan int), nonce, keys.public, keys.secret)
	assert.Error(t, err, "a payload that cannot be encoded must be reported")
}

func TestOpenRejectsWhatIsNotBase64(t *testing.T) {
	keys, err := newKeyPair()
	require.NoError(t, err, "newKeyPair")
	var nonce [nonceLen]byte
	var out map[string]any
	assert.Error(t, open("not base64!!", nonce, keys.public, keys.secret, &out),
		"a message that is not base64 must be refused")
}

func TestOpenRejectsAPayloadOfTheWrongShape(t *testing.T) {
	keys, err := newKeyPair()
	require.NoError(t, err, "newKeyPair")
	var nonce [nonceLen]byte
	sealed, err := seal("a bare string", nonce, keys.public, keys.secret)
	require.NoError(t, err, "seal")
	var out map[string]any
	assert.Error(t, open(sealed, nonce, keys.public, keys.secret, &out),
		"a payload that decrypts but does not fit must be reported")
}

func TestKeyExchangeWithNoKeyIsAnError(t *testing.T) {
	s := newFakeServer(t)
	s.prelude[actionChangePublicKeys] = []envelope{{
		Action:  actionChangePublicKeys,
		Success: "true",
	}}
	_, err := Connect(s.dial(), time.Second, time.Second)
	require.Error(t, err, "a key exchange that returned no key must not be treated as done")
	assert.ErrorContains(t, err, "no public key", "the error must say the key was missing")
}

func TestReplyWithNoMessageIsAnError(t *testing.T) {
	s := newFakeServer(t)
	s.prelude[actionGetLogins] = []envelope{{
		Action:  actionGetLogins,
		Success: "true",
	}}
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	require.Error(t, err, "a reply carrying nothing to decrypt must be an error")
	assert.ErrorContains(t, err, "no message", "the error must say the message was missing")
}

func TestNamedFailureIsReportedAsGiven(t *testing.T) {
	s := newFakeServer(t)
	s.failWith[actionGetLogins] = envelope{Success: "false", Error: "something specific"}
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	assert.ErrorContains(t, err, "something specific", "KeePassXC's own words must be passed through")
}

func TestUnnamedFailureStillReportsTheAction(t *testing.T) {
	s := newFakeServer(t)
	// A refusal KeePassXC does not put a name to arrives inside the encrypted
	// message, which is where acceptance is stated at all; the envelope carries
	// a name only when there is one.
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "false"}
	}
	c := s.connect()
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	assert.ErrorContains(t, err, actionGetLogins, "the failing action must be named")
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
	_, err := Connect(conn, time.Second, time.Second)
	assert.Error(t, err, "a deadline that cannot be set must not be ignored — it is what bounds the wait")
}

func TestAWriteThatFailsIsReported(t *testing.T) {
	s := newFakeServer(t)
	conn := &brokenConn{Conn: s.dial(), failWrite: true}
	_, err := Connect(conn, time.Second, time.Second)
	require.Error(t, err, "a request that could not be sent must be reported")
	// Naming the send is what distinguishes this from the failure that arrives
	// anyway when the write is ignored: the reply never comes, and the deadline
	// reports that instead — a whole timeout later, about the wrong thing.
	assert.ErrorContains(t, err, "sending", "the error must say the request could not be sent")
}

func TestAServerThatHangsUpIsReported(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()
	// Closing the listener's connection from our side is the closest a test
	// gets to KeePassXC exiting mid-session.
	require.NoError(t, c.conn.Close(), "closing")
	_, err := c.GetLogins("ssh://k", Association{ID: "db", IDKey: "key"})
	assert.Error(t, err, "a session whose peer went away must fail, not wait")
}

func TestConnectDefaultsItsTimeouts(t *testing.T) {
	s := newFakeServer(t)
	c, err := Connect(s.dial(), 0, 0)
	require.NoError(t, err, "Connect")
	assert.Equal(t, DefaultTimeout, c.timeout, "a caller that named no timeout gets DefaultTimeout")
	assert.Equal(t, DefaultInteractiveTimeout, c.interactive, "a caller that named none gets DefaultInteractiveTimeout")
}

// TestAssociateWaitsLongerThanAnOrdinaryExchange pins the distinction the two
// budgets exist for: the association dialog is answered by a person, and a
// person is slower than a socket. Held to the ordinary budget, the approval
// would be abandoned while the user was still reading it.
func TestAssociateWaitsLongerThanAnOrdinaryExchange(t *testing.T) {
	s := newFakeServer(t)
	// Long enough that a request held to it would not have expired by the time
	// the reply arrives, and short enough that the test does not wait on it.
	s.replyAfter = map[string]time.Duration{
		actionAssociate: 120 * time.Millisecond,
		actionGetLogins: 120 * time.Millisecond,
	}
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "true", "id": "db"}
	}
	c, err := Connect(s.dial(), 30*time.Millisecond, 5*time.Second)
	require.NoError(t, err, "Connect")

	_, err = c.Associate()
	assert.NoError(t, err, "Associate must have used the slower budget")
	_, err = c.GetLogins("sshakku://k", Association{ID: "db", IDKey: "key"})
	assert.Error(t, err, "an ordinary exchange must still be held to the ordinary budget")
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
				filepath.Join("/var/folders/ab/T", socketName),
				filepath.Join("/tmp", socketName),
			},
		},
		{
			name:       "Linux prefers the per-application runtime directory",
			goos:       "linux",
			tempDir:    "/tmp",
			runtimeDir: "/run/user/1000",
			want: []string{
				filepath.Join("/run/user/1000/app/org.keepassxc.KeePassXC", socketName),
				filepath.Join("/run/user/1000", socketName),
				filepath.Join("/tmp", socketName),
			},
		},
		{
			name:       "Linux with no runtime directory has only the fallback",
			goos:       "linux",
			tempDir:    "/tmp",
			runtimeDir: "",
			want:       []string{filepath.Join("/tmp", socketName)},
		},
		{
			name:       "macOS whose temporary directory is /tmp does not repeat it",
			goos:       "darwin",
			tempDir:    "/tmp",
			runtimeDir: "",
			want:       []string{filepath.Join("/tmp", socketName)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, socketPaths(tc.goos, tc.tempDir, tc.runtimeDir), "socketPaths")
		})
	}
}

// TestSocketPathsUsesTheRunningPlatform proves the exported wrapper actually
// consults the environment rather than returning a fixed list.
func TestSocketPathsUsesTheRunningPlatform(t *testing.T) {
	paths := SocketPaths()
	require.NotEmpty(t, paths, "SocketPaths must always offer at least the fallback")
	assert.Equal(t, filepath.Join("/tmp", socketName), paths[len(paths)-1],
		"the last candidate must be the /tmp fallback")
}
