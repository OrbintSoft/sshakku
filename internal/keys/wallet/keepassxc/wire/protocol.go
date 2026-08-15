// Package wire speaks the local protocol KeePassXC exposes for its browser
// extension: JSON messages over a unix socket, every one of them encrypted with
// NaCl box, and knows where to find the socket they go to.
//
// It talks to a KeePassXC that is already running with its database unlocked,
// so it asks the user for nothing — the same precondition, and the same silent
// outcome, as reaching KeePassXC through the freedesktop Secret Service on
// Linux. The user has to enable "Browser Integration" in KeePassXC's settings
// and approve the association once; after that the stored identification key
// identifies this client on every later connection.
//
// The protocol is defined by KeePassXC itself, not by this project. Field names
// and the nonce rule below are its wire format and must match it exactly.
package wire

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/nacl/box"
)

// nonceLen is the NaCl box nonce width. The protocol carries it base64-encoded.
const nonceLen = 24

// keyLen is the width of an X25519 public or secret key.
const keyLen = 32

// Protocol actions. These strings are KeePassXC's, not ours.
const (
	actionChangePublicKeys = "change-public-keys"
	actionAssociate        = "associate"
	actionTestAssociate    = "test-associate"
	actionGetLogins        = "get-logins"
	actionSetLogin         = "set-login"
)

// ErrNotAssociated reports that KeePassXC does not recognise our identification
// key: either this client has never been associated, or the user revoked it.
// The caller's answer is to associate again, which needs the user's approval.
var ErrNotAssociated = errors.New("keepassxc: this client is not associated with the open database")

// ErrDatabaseLocked reports that KeePassXC is running but its database is
// locked, so it can answer nothing until the user unlocks it. Distinguished
// from a missing entry: a locked database is not the same as "no such
// passphrase", and treating it as a miss would overwrite a stored entry.
var ErrDatabaseLocked = errors.New("keepassxc: the database is locked")

// envelope is the outer, unencrypted frame every message travels in. The
// encrypted payload rides in Message; Action is repeated outside so the peer
// can route the frame without decrypting it first.
type envelope struct {
	Action   string `json:"action"`
	Message  string `json:"message,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
	ClientID string `json:"clientID,omitempty"`

	// change-public-keys carries the public key in the clear: it is the
	// exchange that makes encryption possible, so it cannot itself be
	// encrypted.
	PublicKey string `json:"publicKey,omitempty"`

	// Set by KeePassXC on a failure it can name.
	Success string `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"errorCode,omitempty"`
}

// errorCode values KeePassXC returns that the caller must tell apart. Every
// other code is reported as a plain error.
const (
	errCodeDatabaseNotOpened = "1"
	errCodeAssociationFailed = "4"
	errCodeNoLogins          = "15"
)

// errNoLogins is how KeePassXC reports that a URL matched nothing. It says so
// as an error rather than by answering with an empty list, so the one caller
// that asks — GetLogins — turns it back into the empty list its callers expect.
var errNoLogins = errors.New("keepassxc: no entry matches that URL")

// replyStatus is the acceptance flag every encrypted reply carries **inside**
// the encrypted message. The envelope wrapped around it does not repeat it: on
// the wire, only a failure is named outside, and a reply that succeeded says so
// only once it has been decrypted.
type replyStatus struct {
	Success string `json:"success"`
}

// succeeded reports whether KeePassXC accepted the request.
func (s replyStatus) succeeded() bool { return s.Success == "true" }

// encryptedReply is a reply whose acceptance can only be read after decrypting
// it. Every encrypted reply type embeds replyStatus to satisfy this.
type encryptedReply interface {
	succeeded() bool
}

// randReader is the entropy every key and nonce here comes from. It is a seam
// so the failure branches below — which the real reader does not take — can be
// exercised rather than assumed.
var randReader io.Reader = rand.Reader

// newNonce returns a fresh random nonce.
func newNonce() ([nonceLen]byte, error) {
	var n [nonceLen]byte
	if _, err := io.ReadFull(randReader, n[:]); err != nil {
		return n, fmt.Errorf("keepassxc: generating a nonce: %w", err)
	}
	return n, nil
}

// incrementNonce returns the nonce KeePassXC answers a request with: the
// request's own nonce plus one, as a little-endian 24-byte integer. Checking it
// is what ties a reply to the request that asked for it, so a reply carrying
// any other nonce is refused rather than trusted.
func incrementNonce(n [nonceLen]byte) [nonceLen]byte {
	carry := 1
	for i := range n {
		carry += int(n[i])
		n[i] = byte(carry)
		carry >>= 8
	}
	return n
}

// keyPair is one X25519 key pair.
type keyPair struct {
	public *[keyLen]byte
	secret *[keyLen]byte
}

// newKeyPair generates a fresh X25519 key pair.
func newKeyPair() (keyPair, error) {
	pub, priv, err := box.GenerateKey(randReader)
	if err != nil {
		return keyPair{}, fmt.Errorf("keepassxc: generating a key pair: %w", err)
	}
	return keyPair{public: pub, secret: priv}, nil
}

// decodeKey parses a base64 key of exactly keyLen bytes. A key of any other
// width is refused rather than padded or truncated: silently reshaping key
// material would produce a client that encrypts to nobody.
func decodeKey(encoded string) (*[keyLen]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("keepassxc: decoding a key: %w", err)
	}
	if len(raw) != keyLen {
		return nil, fmt.Errorf("keepassxc: a key must be %d bytes, got %d", keyLen, len(raw))
	}
	var key [keyLen]byte
	copy(key[:], raw)
	return &key, nil
}

// seal encrypts payload as JSON to the peer's public key and returns it
// base64-encoded, ready for an envelope's Message field.
func seal(payload any, nonce [nonceLen]byte, peer, secret *[keyLen]byte) (string, error) {
	plain, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("keepassxc: encoding a request: %w", err)
	}
	return base64.StdEncoding.EncodeToString(box.Seal(nil, plain, &nonce, peer, secret)), nil
}

// open decrypts an envelope's Message into out. The nonce is the caller's
// expected one rather than the one the reply carries, so a reply encrypted
// under a different nonce fails to open instead of being accepted.
func open(encoded string, nonce [nonceLen]byte, peer, secret *[keyLen]byte, out any) error {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("keepassxc: decoding a reply: %w", err)
	}
	plain, ok := box.Open(nil, raw, &nonce, peer, secret)
	if !ok {
		return errors.New("keepassxc: a reply did not decrypt — wrong key or tampered message")
	}
	if err := json.Unmarshal(plain, out); err != nil {
		return fmt.Errorf("keepassxc: decoding a reply: %w", err)
	}
	return nil
}
