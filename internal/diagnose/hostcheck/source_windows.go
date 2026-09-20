//go:build windows

package hostcheck

import (
	"context"
)

// Windows gathers the host-hardening observations on this system.
//
// One of the three is answered here: whether the disk holding Target is
// encrypted, which BitLocker is what does on this platform. The other two are
// not. The temporary directory question is about a filesystem held in memory,
// which is a thing this system does not have, and the hardware one has an
// answer here — a TPM — that this build does not go and read. Both stay
// undetermined rather than reported as a definite no, which would describe a
// machine with nothing protecting it rather than one nobody asked.
//
// Target is the path whose backing disk the encryption question is about.
type Windows struct {
	Target string
}

// Checks reports what this system was asked and answers the rest as
// undetermined.
//
// A disk that could not be asked about is undetermined too: a volume with no
// drive letter, a shell that would not speak about it, a property it does not
// carry. None of those is evidence that the disk is in the clear.
func (w Windows) Checks(ctx context.Context) Checks {
	if ctx.Err() != nil {
		return Checks{}
	}
	root, ok := volumeRootOf(w.Target)
	if !ok {
		return Checks{}
	}
	encrypted, err := volumeProtection(root)
	if err != nil {
		return Checks{}
	}
	return Checks{DiskEncrypted: encrypted, DiskEncryptionKind: "BitLocker"}
}

var _ Source = Windows{}
