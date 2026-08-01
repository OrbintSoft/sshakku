//go:build darwin

package keys

import "time"

// platformBlockingCases adds nothing on macOS: the wallet this platform uses by
// default is the Keychain, which SSHakku calls into in-process rather than by
// running a program, so there is no command here for this test to block.
//
// That is not the same as the case being covered. Nothing bounds that call
// either — see the keychain row in docs/TEST-MATRIX.md, which records it as a
// violation of F21 rather than as a case outside it.
func platformBlockingCases(time.Duration) []blockingCase { return nil }
