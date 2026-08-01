//go:build darwin

package keys

import (
	"errors"
	"testing"
	"time"
)

// blockingKeychainClient is a keychain that neither answers nor fails: every
// call waits until the test lets it go. A real keychain does this when the
// framework is waiting on an authorization nobody is there to grant, and it is
// the one thing a real one cannot be asked to do on demand.
type blockingKeychainClient struct {
	release chan struct{}
}

// newBlockingKeychainClient releases whatever is still blocked when the test
// ends: a call abandoned by its caller stays in flight, and one left in flight
// past the suite is a goroutine goleak will rightly fail the build over.
func newBlockingKeychainClient(t *testing.T) *blockingKeychainClient {
	t.Helper()
	c := &blockingKeychainClient{release: make(chan struct{})}
	t.Cleanup(func() { close(c.release) })
	return c
}

func (c *blockingKeychainClient) Add(_, _, _, _ string) error { <-c.release; return nil }
func (c *blockingKeychainClient) Update(_, _, _ string) error { <-c.release; return nil }
func (c *blockingKeychainClient) Delete(_, _ string) error    { <-c.release; return nil }

func (c *blockingKeychainClient) Find(_, _ string) (string, bool, error) {
	<-c.release
	return "", false, nil
}

func (c *blockingKeychainClient) List(_ string) ([]string, error) {
	<-c.release
	return nil, nil
}

var _ KeychainClient = (*blockingKeychainClient)(nil)

// TestKeychainGivesUpOnAKeychainThatNeverAnswers verifies F21 for the only
// store sshakku calls into directly instead of running: every other wallet is
// reached by a child process the runner can kill, and the keychain is not.
// Whichever operation is in flight, something is waiting on the answer — a
// login shell, or an ssh at a passphrase prompt — so none of them may wait
// without end.
func TestKeychainGivesUpOnAKeychainThatNeverAnswers(t *testing.T) {
	ops := []struct {
		name string
		call func(*KeychainBackend) error
	}{
		{"Lookup", func(b *KeychainBackend) error { _, _, err := b.Lookup("svc"); return err }},
		{"Store", func(b *KeychainBackend) error { return b.Store("svc", "a key", "hunter2") }},
		{"Delete", func(b *KeychainBackend) error { return b.Delete("svc") }},
		{"List", func(b *KeychainBackend) error { _, err := b.List(); return err }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			b := &KeychainBackend{
				Client:  newBlockingKeychainClient(t),
				Account: "alice",
				Timeout: 100 * time.Millisecond,
			}

			done := make(chan error, 1)
			go func() { done <- op.call(b) }()

			// Twenty times the budget: what is being judged is that the wait
			// ends at all, not how punctually.
			select {
			case err := <-done:
				if !errors.Is(err, ErrTimedOut) {
					t.Fatalf("%s = %v; a keychain that never answered must be reported as not having answered", op.name, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s never returned: a keychain that neither answers nor fails holds the shell for as long as it likes", op.name)
			}
		})
	}
}
