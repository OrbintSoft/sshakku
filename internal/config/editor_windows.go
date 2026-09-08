//go:build windows

package config

// platformEditor is the editor to open a file in where nobody named one.
// Notepad is part of this system rather than something installed on it, so it
// is there to be run on an account nobody has set up — which is the only claim
// worth making about an editor the user was never asked for.
const platformEditor = "notepad.exe"
