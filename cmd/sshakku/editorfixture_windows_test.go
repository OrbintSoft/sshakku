//go:build windows

package main

// editorFixtureName is the stand-in editor this system can run — see
// useEditor. A shell script is not executable here: CreateProcess refuses one
// with "%1 is not a valid Win32 application", since nothing on this system
// reads the interpreter line at the top. A batch file is what a program can
// start by name.
const editorFixtureName = "editor.cmd"
