//go:build unix

package install

import "os"

// makeDirectoryTheMachineShares makes the directory a machine-wide install's
// hook goes in.
//
// On this system it goes under a prefix that belongs to the account a
// machine-wide install has to be made by, and that no other account may write
// in, so what a directory made there permits is already the answer wanted: the
// mode says every account may read it, and where it sits says who may not write
// it.
func makeDirectoryTheMachineShares(dir string) error {
	return os.MkdirAll(dir, hookDirMode)
}
