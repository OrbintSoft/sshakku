//go:build windows

package diagnose

import "context"

// WindowsHostSource gathers the host-hardening observations on this system —
// none of them yet. Every field of the zero HostChecks means "could not
// determine", which is what is true here: the questions have Windows answers
// (BitLocker for the disk, a TPM for the hardware key store), and this build
// asks neither, so it says nothing rather than reporting a definite "no" that
// would read as a machine with no protection at all.
//
// Target is the path whose backing disk the encryption question is about, kept
// so the source is constructed the same way on every platform.
type WindowsHostSource struct {
	Target string
}

// Checks reports everything as undetermined.
func (WindowsHostSource) Checks(context.Context) HostChecks { return HostChecks{} }

var _ HostSource = WindowsHostSource{}
