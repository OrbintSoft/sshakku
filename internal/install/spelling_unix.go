//go:build unix

package install

// spellingFor returns how paths must be written for the shell at interpreter.
//
// They are written as they are. This system is the one a POSIX-emulating
// environment emulates, so a shell here and this program spell a path the same
// way and there is nothing between them to translate — which is an answer, not
// a step that was skipped.
func spellingFor(string) spelling {
	return spelling{}
}
