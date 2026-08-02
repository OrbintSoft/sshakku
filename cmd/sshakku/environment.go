package main

import (
	"os"

	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// shownEnvVars names every environment variable SSHakku reads whose value can
// be shown: the agent and askpass wiring a login shell exports, then the
// settings that override config.toml. An override is invisible in the
// configuration file it overrides, so a report that omitted these would leave
// a reader comparing the file against behaviour it does not explain.
var shownEnvVars = []string{
	"SSH_AUTH_SOCK",
	"SSH_ASKPASS",
	"SSH_ASKPASS_REQUIRE",
	"SSHAKKU_KEY_LIFETIME",
	"SSHAKKU_GIVEUP_TTL",
	"SSHAKKU_COMMAND_TIMEOUT",
	"SSHAKKU_INTERACTIVE_TIMEOUT",
	"SSHAKKU_MAX_ATTEMPTS",
	"SSHAKKU_NO_GIVEUP",
	"SSHAKKU_QUIET",
}

// secretEnvVars names the variables whose value must never be printed. The
// handoff token redeems a passphrase in transit; the Bitwarden variable is one
// SSHakku writes into `bw`'s environment rather than reads, but the name means
// "master password" here and someone may well export it believing it to be a
// setting. Either way the report only ever says whether it is set.
var secretEnvVars = []string{
	keys.EnvPassHandoffToken,
	keys.EnvBitwardenPassword,
}

// environmentReport reads the variables above from this process's environment.
func environmentReport() ([]diagnose.EnvVar, []diagnose.SecretEnvVar) {
	shown := make([]diagnose.EnvVar, 0, len(shownEnvVars))
	for _, name := range shownEnvVars {
		shown = append(shown, diagnose.EnvVar{Name: name, Value: os.Getenv(name)})
	}
	secrets := make([]diagnose.SecretEnvVar, 0, len(secretEnvVars))
	for _, name := range secretEnvVars {
		_, set := os.LookupEnv(name)
		secrets = append(secrets, diagnose.SecretEnvVar{Name: name, Set: set})
	}
	return shown, secrets
}

// environmentNames returns the same variables with nothing filled in, for a
// report about a session whose environment this process cannot read. The names
// are still worth showing — they say what the report would have covered — but
// every value is withheld, which is what diagnose.Inputs.EnvUnreadable makes
// the report say out loud instead of printing them as unset.
func environmentNames() ([]diagnose.EnvVar, []diagnose.SecretEnvVar) {
	shown := make([]diagnose.EnvVar, 0, len(shownEnvVars))
	for _, name := range shownEnvVars {
		shown = append(shown, diagnose.EnvVar{Name: name})
	}
	secrets := make([]diagnose.SecretEnvVar, 0, len(secretEnvVars))
	for _, name := range secretEnvVars {
		secrets = append(secrets, diagnose.SecretEnvVar{Name: name})
	}
	return shown, secrets
}
