//go:build windows

package keys

import "time"

// platformBlockingTools names no extra program: this build runs none of its
// own beyond the wallets every platform shares.
func platformBlockingTools() []string { return nil }

// platformBlockingCases adds nothing here. The wallet this platform would use
// by default is the Credential Manager, which sshakku does not open at all yet,
// so there is no command of its own for this test to block.
func platformBlockingCases(time.Duration) []blockingCase { return nil }
