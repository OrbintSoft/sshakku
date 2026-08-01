//go:build darwin

// The freedesktop Secret Service is a Linux D-Bus protocol
// (org.freedesktop.secrets); there is no equivalent to talk to off Linux, so
// this package exposes no client there. Platforms without it use their own
// native secret store instead (the macOS keychain — see cmd/sshakku's
// non-Linux secret-backend wiring).
package secretservice
