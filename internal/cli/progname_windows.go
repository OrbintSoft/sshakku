//go:build windows

package cli

// programSuffix is what this system puts at the end of a program's file name.
// A file without it is not a program here: nothing will execute it, and the
// helper ssh is pointed at has to be one.
const programSuffix = ".exe"
