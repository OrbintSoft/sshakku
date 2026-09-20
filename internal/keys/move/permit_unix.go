//go:build unix

package move

import (
	"fmt"
	"os"
)

// Permit gives one path the mode this system expects of it: a key directory
// that only its owner may enter, a private key that only its owner may read,
// and a public half that anybody may.
//
// OpenSSH here refuses a private key that is readable by anyone else, and says
// so at the moment somebody is trying to connect rather than when the key was
// put there. The public half is deliberately not hidden: it is a public key,
// and a tool that reads it to work out which key to offer is doing its job.
func Permit(path string, kind Kind) error {
	var mode os.FileMode
	switch kind {
	case Directory:
		mode = 0o700
	case PrivateKey:
		mode = 0o600
	case PublicKey:
		mode = 0o644
	default:
		return fmt.Errorf("permitting %s: %d is not a kind of path this knows about", path, kind)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("permitting %s: %w", path, err)
	}
	return nil
}
