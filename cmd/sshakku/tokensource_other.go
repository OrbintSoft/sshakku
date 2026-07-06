//go:build !linux

package main

// execTokenSource has nothing to read on platforms without a Linux kernel
// keyring: paths.SocketToken/ReadSocketToken already degrade to "" there, so no
// privilege transition is ever needed to read another user's token.
type execTokenSource struct{}

var _ TargetTokenSource = execTokenSource{}

func (execTokenSource) ReadToken(int, int) (string, error) { return "", nil }
