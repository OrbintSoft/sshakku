package keepassxc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// DefaultTimeout bounds a single exchange with KeePassXC. The socket is local
// and an unlocked database answers immediately, so this is generous: it exists
// so a KeePassXC that accepts the connection and then says nothing cannot hold
// a shell open, not to rush a healthy one.
const DefaultTimeout = 10 * time.Second

// Association is what survives between runs: KeePassXC recognises a client by
// the identification key it associated, and names that association with an id.
// Losing it costs the user another approval dialog, so the caller persists it.
type Association struct {
	// ID is the name KeePassXC gave this association.
	ID string
	// IDKey is the identification public key, base64-encoded, that KeePassXC
	// knows this client by. It is a public key: it identifies, it does not
	// decrypt, so it is not a secret in the sense the passphrases are.
	IDKey string
}

// Entry is one credential KeePassXC returned.
type Entry struct {
	Login    string `json:"login"`
	Name     string `json:"name"`
	Password string `json:"password"`
	UUID     string `json:"uuid"`
}

// Client is a connected, key-exchanged session with KeePassXC. Not safe for
// concurrent use: one request is in flight at a time, and each reply is matched
// to its request by nonce.
type Client struct {
	conn    net.Conn
	timeout time.Duration
	// dec lives as long as the connection. A decoder buffers whatever it read
	// past the message it returned, so building a new one per exchange would
	// discard the start of the next reply.
	dec      *json.Decoder
	clientID string
	session  keyPair
	hostKey  *[keyLen]byte
}

// Connect performs the key exchange over conn and returns a session ready to
// carry encrypted messages. It takes ownership of conn: Close closes it.
//
// timeout bounds every later exchange as well as this one; zero selects
// DefaultTimeout.
func Connect(conn net.Conn, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	session, err := newKeyPair()
	if err != nil {
		return nil, err
	}
	id, err := newNonce()
	if err != nil {
		return nil, err
	}
	c := &Client{
		conn:     conn,
		timeout:  timeout,
		dec:      json.NewDecoder(conn),
		clientID: base64.StdEncoding.EncodeToString(id[:]),
		session:  session,
	}
	if err := c.changePublicKeys(); err != nil {
		return nil, err
	}
	return c, nil
}

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

// changePublicKeys swaps session public keys in the clear — the one exchange
// that cannot be encrypted, because it is what establishes the encryption.
func (c *Client) changePublicKeys() error {
	nonce, err := newNonce()
	if err != nil {
		return err
	}
	req := envelope{
		Action:    actionChangePublicKeys,
		PublicKey: base64.StdEncoding.EncodeToString(c.session.public[:]),
		Nonce:     base64.StdEncoding.EncodeToString(nonce[:]),
		ClientID:  c.clientID,
	}
	reply, err := c.exchange(req, actionChangePublicKeys)
	if err != nil {
		return err
	}
	if reply.PublicKey == "" {
		return errors.New("keepassxc: the key exchange returned no public key")
	}
	hostKey, err := decodeKey(reply.PublicKey)
	if err != nil {
		return err
	}
	c.hostKey = hostKey
	return nil
}

// associateRequest is the inner, encrypted payload of an associate call.
type associateRequest struct {
	Action string `json:"action"`
	Key    string `json:"key"`
	IDKey  string `json:"idKey"`
}

// associateReply is what associate and test-associate answer with.
type associateReply struct {
	Success string `json:"success"`
	ID      string `json:"id"`
	Hash    string `json:"hash"`
	Nonce   string `json:"nonce"`
	Error   string `json:"error"`
}

// Associate asks KeePassXC to remember this client. It raises a dialog the user
// must approve, so it is the one call that is not silent — the caller reaches it
// only when TestAssociate has already reported that no usable association
// exists.
//
// The returned Association is what makes every later run silent; persisting it
// is the caller's job.
func (c *Client) Associate() (Association, error) {
	ident, err := newKeyPair()
	if err != nil {
		return Association{}, err
	}
	idKey := base64.StdEncoding.EncodeToString(ident.public[:])
	var reply associateReply
	err = c.request(actionAssociate, associateRequest{
		Action: actionAssociate,
		Key:    base64.StdEncoding.EncodeToString(c.session.public[:]),
		IDKey:  idKey,
	}, &reply)
	if err != nil {
		return Association{}, err
	}
	if reply.ID == "" {
		return Association{}, errors.New("keepassxc: the association was accepted but named no database")
	}
	return Association{ID: reply.ID, IDKey: idKey}, nil
}

// testAssociateRequest is the inner payload of a test-associate call.
type testAssociateRequest struct {
	Action string `json:"action"`
	ID     string `json:"id"`
	Key    string `json:"key"`
}

// TestAssociate reports whether KeePassXC still recognises a stored
// association. A rejected one comes back as ErrNotAssociated rather than a
// generic failure, because the answer to it is specific: associate again.
func (c *Client) TestAssociate(a Association) error {
	var reply associateReply
	return c.request(actionTestAssociate, testAssociateRequest{
		Action: actionTestAssociate,
		ID:     a.ID,
		Key:    a.IDKey,
	}, &reply)
}

// associationKey names one stored association in a get-logins call.
type associationKey struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// getLoginsRequest is the inner payload of a get-logins call.
type getLoginsRequest struct {
	Action string           `json:"action"`
	URL    string           `json:"url"`
	Keys   []associationKey `json:"keys"`
}

// getLoginsReply is what get-logins answers with.
type getLoginsReply struct {
	Count   int     `json:"count"`
	Entries []Entry `json:"entries"`
	Success string  `json:"success"`
	Nonce   string  `json:"nonce"`
}

// GetLogins returns every entry KeePassXC matches to url. No match is an empty
// slice and no error: the caller reads that as "nothing stored yet", which is a
// normal state, not a failure.
func (c *Client) GetLogins(url string, a Association) ([]Entry, error) {
	var reply getLoginsReply
	err := c.request(actionGetLogins, getLoginsRequest{
		Action: actionGetLogins,
		URL:    url,
		Keys:   []associationKey{{ID: a.ID, Key: a.IDKey}},
	}, &reply)
	if err != nil {
		return nil, err
	}
	return reply.Entries, nil
}

// setLoginRequest is the inner payload of a set-login call. Nonce is repeated
// inside the encrypted body: the protocol puts it in both places.
type setLoginRequest struct {
	Action    string `json:"action"`
	URL       string `json:"url"`
	SubmitURL string `json:"submitUrl"`
	ID        string `json:"id"`
	Nonce     string `json:"nonce"`
	Login     string `json:"login"`
	Password  string `json:"password"`
	UUID      string `json:"uuid,omitempty"`
	Group     string `json:"group,omitempty"`
}

// setLoginReply is what set-login answers with.
type setLoginReply struct {
	Success string `json:"success"`
	Nonce   string `json:"nonce"`
	Error   string `json:"error"`
}

// SetLogin stores or updates the entry for url. Passing uuid updates that entry
// in place; an empty uuid creates one. Updating in place is what keeps a
// re-stored passphrase from leaving a second entry behind every time.
func (c *Client) SetLogin(url, login, password, uuid, group string, a Association) error {
	nonce, err := newNonce()
	if err != nil {
		return err
	}
	var reply setLoginReply
	return c.request(actionSetLogin, setLoginRequest{
		Action:    actionSetLogin,
		URL:       url,
		SubmitURL: url,
		ID:        a.ID,
		Nonce:     base64.StdEncoding.EncodeToString(nonce[:]),
		Login:     login,
		Password:  password,
		UUID:      uuid,
		Group:     group,
	}, &reply)
}

// request seals payload, sends it, and decrypts the reply into out.
func (c *Client) request(action string, payload any, out any) error {
	if c.hostKey == nil {
		return errors.New("keepassxc: no key exchange has happened yet")
	}
	nonce, err := newNonce()
	if err != nil {
		return err
	}
	sealed, err := seal(payload, nonce, c.hostKey, c.session.secret)
	if err != nil {
		return err
	}
	reply, err := c.exchange(envelope{
		Action:   action,
		Message:  sealed,
		Nonce:    base64.StdEncoding.EncodeToString(nonce[:]),
		ClientID: c.clientID,
	}, action)
	if err != nil {
		return err
	}
	if reply.Message == "" {
		return fmt.Errorf("keepassxc: %s returned no message", action)
	}
	// The reply is encrypted under the request's nonce plus one. Opening it
	// with anything else fails, which is what rejects a reply that was not
	// produced for this request.
	return open(reply.Message, incrementNonce(nonce), c.hostKey, c.session.secret, out)
}

// exchange writes req and returns the first reply carrying want as its action.
//
// KeePassXC also pushes unsolicited frames — it announces the database being
// locked or unlocked whenever it happens, not only when asked. Those arrive
// interleaved with replies, so anything that is not the awaited action is
// skipped rather than mistaken for the answer.
func (c *Client) exchange(req envelope, want string) (envelope, error) {
	deadline := time.Now().Add(c.timeout)
	if err := c.conn.SetDeadline(deadline); err != nil {
		return envelope{}, fmt.Errorf("keepassxc: setting a deadline: %w", err)
	}
	raw, err := json.Marshal(req)
	// An envelope holds nothing but strings, so encoding one cannot fail. The
	// error is still checked rather than discarded, in case the type grows a
	// field that can.
	//coverage:ignore
	if err != nil {
		return envelope{}, fmt.Errorf("keepassxc: encoding a request: %w", err)
	}
	if _, err := c.conn.Write(raw); err != nil {
		return envelope{}, fmt.Errorf("keepassxc: sending %s: %w", req.Action, err)
	}

	for {
		var reply envelope
		if err := c.dec.Decode(&reply); err != nil {
			return envelope{}, fmt.Errorf("keepassxc: reading the reply to %s: %w", req.Action, err)
		}
		if reply.Action != want {
			continue
		}
		if err := replyError(reply); err != nil {
			return envelope{}, err
		}
		return reply, nil
	}
}

// replyError turns a failure KeePassXC names into the error the caller can act
// on. A locked database and a refused association each get their own error
// because each has its own answer; anything else is reported as it came.
func replyError(reply envelope) error {
	if reply.Success == "true" {
		return nil
	}
	switch reply.Code {
	case errCodeDatabaseNotOpened:
		return ErrDatabaseLocked
	case errCodeAssociationFailed:
		return ErrNotAssociated
	}
	if reply.Error != "" {
		return fmt.Errorf("keepassxc: %s failed: %s", reply.Action, reply.Error)
	}
	return fmt.Errorf("keepassxc: %s failed", reply.Action)
}
