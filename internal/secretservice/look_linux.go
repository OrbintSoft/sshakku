//go:build linux

package secretservice

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/godbus/dbus/v5"
)

// Look is what the session bus and, when one is already there, the wallet on it
// answer about themselves. Everything here is learned by asking questions whose
// answers are names and paths: no secret is read, nothing is created, and a
// wallet that is not running is not started.
type Look struct {
	// Running is whether a wallet answers on this bus now.
	Running bool
	// Activatable is whether the bus knows how to start one, when none is
	// answering yet.
	Activatable bool
	// CollectionFound is whether the compartment asked about is already there.
	// It says nothing unless Running.
	CollectionFound bool
	// AskErr is set when there was a wallet to ask and it did not answer, so a
	// caller can tell "it is not there" from "it could not be told".
	AskErr error
}

// LookForCollection asks the bus whether a wallet is reachable and, only when
// one is already answering, asks that wallet whether the compartment named by
// alias — or, on an implementation that supports no alias but its own, by
// label — already exists.
//
// A wallet that is merely activatable is left alone: starting it would be an
// act, and this is a look. Every round-trip is bounded by timeout, so a wallet
// that has stopped answering costs the caller that much delay and no more.
func LookForCollection(ctx context.Context, alias, label string, timeout time.Duration) (Look, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return Look{}, fmt.Errorf("secret service: connect session bus: %w", err)
	}
	defer func() { _ = conn.Close() }()

	names, err := listBusNames(ctx, conn.BusObject(), timeout, "org.freedesktop.DBus.ListNames")
	if err != nil {
		return Look{}, fmt.Errorf("secret service: list bus names: %w", err)
	}

	look := Look{Running: slices.Contains(names, busName)}
	if look.Running && alias == "" && label == "" {
		// A caller with no compartment to ask about: whether a wallet is there
		// is the whole question.
		return look, nil
	}
	if !look.Running {
		activatable, err := listBusNames(ctx, conn.BusObject(), timeout, "org.freedesktop.DBus.ListActivatableNames")
		if err != nil {
			return look, fmt.Errorf("secret service: list activatable bus names: %w", err)
		}
		look.Activatable = slices.Contains(activatable, busName)
		return look, nil
	}

	// A client with no session: a session exists to carry secrets, and asking
	// for one would be asking the wallet to make something. Its call timeout is
	// the one given here, which is shorter than a stored passphrase is worth
	// waiting for and about as long as a report may take.
	client := &Client{conn: conn, service: conn.Object(busName, rootPath), CallTimeout: timeout}

	var existing dbus.ObjectPath
	if err := client.call(ctx, client.service, serviceIface+".ReadAlias", alias).Store(&existing); err != nil {
		look.AskErr = fmt.Errorf("secret service: read alias %q: %w", alias, err)
		return look, nil
	}
	if existing != noPrompt {
		look.CollectionFound = true
		return look, nil
	}

	// The alias is unset either because the compartment is not there or because
	// this implementation keeps no aliases but its own, in which case the
	// compartment it made carries the name as its label instead.
	found, err := client.findCollectionByLabel(ctx, label)
	if err != nil {
		look.AskErr = err
		return look, nil
	}
	look.CollectionFound = found != noPrompt
	return look, nil
}

// listBusNames asks the message bus for one of its own name lists. A var, not a
// plain function, so a test can make the bus fail to answer: a live bus that has
// just accepted a connection does not stop answering on request, and what a look
// concludes when it cannot be asked is exactly what has to be checkable.
var listBusNames = func(ctx context.Context, obj dbus.BusObject, timeout time.Duration, method string) ([]string, error) {
	var names []string
	err := lookCall(ctx, obj, timeout, method).Store(&names)
	return names, err
}

// lookCall is one bounded D-Bus round-trip on an object this package holds no
// Client for — the message bus itself, which answers about names rather than
// about secrets.
func lookCall(ctx context.Context, obj dbus.BusObject, timeout time.Duration, method string, args ...interface{}) *dbus.Call {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return obj.CallWithContext(ctx, method, 0, args...)
}
