//go:build windows

package hostcheck

import "context"

// Windows gathers the host-hardening observations on this system —
// none of them yet. Every field of the zero Checks means "could not
// determine", which is what is true here: the questions have Windows answers
// (BitLocker for the disk, a TPM for the hardware key store), and this build
// asks neither, so it says nothing rather than reporting a definite "no" that
// would read as a machine with no protection at all.
//
// Target is the path whose backing disk the encryption question is about, kept
// so the source is constructed the same way on every platform.
type Windows struct {
	Target string
}

// Checks reports everything as undetermined.
func (Windows) Checks(context.Context) Checks { return Checks{} }

var _ Source = Windows{}
