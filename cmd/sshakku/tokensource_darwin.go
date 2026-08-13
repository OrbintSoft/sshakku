//go:build darwin

package main

import "context"

// execTokenSource has nothing to read on platforms without a Linux kernel
// keyring: paths.SocketToken/ReadSocketToken already degrade to "" there, so no
// privilege transition is ever needed to read another user's token.
type execTokenSource struct{}

var _ TargetTokenSource = execTokenSource{}

func (execTokenSource) ReadToken(context.Context, int, int) (string, error) { return "", nil }
