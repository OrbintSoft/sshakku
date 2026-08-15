//go:build windows

package crossuser

import "context"

// Exec has nothing to read on platforms without a Linux kernel
// keyring: paths.SocketToken/ReadSocketToken already degrade to "" there, so no
// privilege transition is ever needed to read another user's token.
type Exec struct{}

var _ Source = Exec{}

func (Exec) ReadToken(context.Context, int, int) (string, error) { return "", nil }
