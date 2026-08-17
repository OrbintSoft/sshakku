//go:build windows

package install

// spellingFor returns how paths must be written for the shell at interpreter.
//
// A translator beside the interpreter is what says the two spellings differ: an
// environment that emulates POSIX here ships one, and a shell of this system's
// own needs none. The translator is asked rather than the mapping reproduced,
// because the mapping is that environment's to decide.
func spellingFor(interpreter string) spelling {
	translator, found := FindCygpath(interpreter)
	if !found {
		return spelling{}
	}
	return spelling{toShell: translator.ToUnix, toUs: translator.ToWindows}
}
