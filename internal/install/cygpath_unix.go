//go:build unix

package install

// cygpathCandidates says where a POSIX-emulating environment keeps its path
// translator. Nowhere here: this system is the thing being emulated, so a shell
// and this program spell a path the same way and there is nothing to translate
// between.
//
// Returning no candidates rather than refusing to compile is deliberate. The
// question "does this environment need its paths translated" is one an install
// may reasonably ask anywhere, and "no, and here is nothing" is a real answer
// to it — unlike the wallet table, where a wrong default would arrive quietly.
func cygpathCandidates(string) []string {
	return nil
}
