package main

import (
	"context"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/config"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/secretservice"
)

// newDefaultSecretBackend resolves the default (secret-service) backend on
// Linux: it opens the native Secret Service client and wraps it in a
// SecretServiceBackend, which unlocks its own dedicated collection only for the
// duration of each lookup/store rather than relying on the desktop's idle
// timeout. If the session bus is unreachable (e.g. a headless session with no
// D-Bus user session) it logs the failure and falls back to SecretToolBackend,
// so a key can still be looked up or stored via the desktop's default
// collection rather than aborting the caller outright.
// secretServiceBudgets hands a Secret Service client the two waits the user
// configured: one for what the daemon answers by itself, one for a prompt the
// desktop puts in front of them.
func secretServiceBudgets(client *secretservice.Client, settings config.Settings) *secretservice.Client {
	client.CallTimeout = settings.CommandTimeout
	client.PromptTimeout = settings.InteractiveTimeout
	return client
}

func newDefaultSecretBackend(ctx context.Context, user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	client, err := secretservice.NewClient(ctx)
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("secret service: %v; falling back to secret-tool", err))
		return keys.SecretToolBackend{Runner: run.ExecRunner{}, User: user, Timeout: settings.CommandTimeout}, func() {}
	}
	// Reached only when the session bus is live; the returned client is a
	// concrete *secretservice.Client that a unit test cannot stand in for
	// without a real D-Bus Secret Service, so this cannot run in a unit test.
	// The fallback above is unit-tested.
	//coverage:ignore
	backend := &keys.SecretServiceBackend{
		Client:    secretServiceBudgets(client, settings),
		User:      user,
		Container: settings.SecretContainer,
	}
	return backend, func() {
		//coverage:ignore
		_ = client.Close(ctx)
	}
}
