//go:build unix

package config

// platformEditor is the editor to open a file in where nobody named one. POSIX
// requires vi of every system this file is built for, which is the only claim
// worth making about an editor the user was never asked for.
const platformEditor = "vi"
