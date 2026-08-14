//go:build unix

package config

// absRoot is where an absolute path starts on this system. A path is absolute
// here as soon as it begins at the root, so that is all it takes.
const absRoot = "/"
