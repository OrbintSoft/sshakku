//go:build !darwin

package keys

import "errors"

// ErrKeychainUnavailable signals that the macOS keychain doesn't exist on
// this platform, so KeychainBackend cannot function here.
var ErrKeychainUnavailable = errors.New("keychain unavailable on this platform")

// NoKeychainClient is the non-darwin KeychainClient: every call reports
// ErrKeychainUnavailable, since there is no keychain to talk to off macOS.
type NoKeychainClient struct{}

// Add reports ErrKeychainUnavailable.
func (NoKeychainClient) Add(account, service, label, passphrase string) error {
	return ErrKeychainUnavailable
}

// Update reports ErrKeychainUnavailable.
func (NoKeychainClient) Update(account, service, passphrase string) error {
	return ErrKeychainUnavailable
}

// Find reports ErrKeychainUnavailable.
func (NoKeychainClient) Find(account, service string) (string, bool, error) {
	return "", false, ErrKeychainUnavailable
}

// Delete reports ErrKeychainUnavailable.
func (NoKeychainClient) Delete(account, service string) error {
	return ErrKeychainUnavailable
}

// List reports ErrKeychainUnavailable.
func (NoKeychainClient) List(account string) ([]string, error) {
	return nil, ErrKeychainUnavailable
}

var _ KeychainClient = NoKeychainClient{}
