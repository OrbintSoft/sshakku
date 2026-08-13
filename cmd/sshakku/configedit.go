package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/config"
)

// fallbackEditor is the editor to run when neither variable names one. POSIX
// requires it of every system, which is the only claim that can be made about
// an editor SSHakku did not ask the user for.
const fallbackEditor = "vi"

// configEdit opens the user's own config.toml in their editor, creating it
// from the commented template when they have none, and reports what the file
// itself cannot show once the editor exits.
func (d deps) configEdit(ctx context.Context, stdout, stderr io.Writer, configDir string) int {
	path := config.MainFile(configDir)
	if err := ensureConfigFile(configDir, path); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	if err := runEditor(path); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return reportEdited(stdout, stderr, configDir, path)
}

// ensureConfigFile puts a file where the user expects to edit one. An editor
// opened on nothing at all leaves someone to remember every setting's name.
func ensureConfigFile(configDir, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", configDir, err)
	}
	if err := os.WriteFile(path, []byte(config.Template()), 0o600); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	return nil
}

// runEditor runs the user's editor on path, with this process's own terminal:
// they are the ones typing into it.
//
// Nothing bounds how long it runs. The limits SSHakku puts on the programs it
// starts are there so a shell is never left waiting on one; here the shell is
// waiting on the person at the keyboard, and cutting their editor short would
// throw away what they had typed.
func runEditor(path string) error {
	command := editorCommand()
	editor := exec.Command(command[0], append(command[1:], path)...) // #nosec G204 -- the editor is the user's own choice, run on their behalf
	editor.Stdin, editor.Stdout, editor.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := editor.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", strings.Join(command, " "), err)
	}
	return nil
}

// editorCommand returns the editor to run and the arguments the user attached
// to it: $EDITOR holds a command line ("code -w", "emacs -nw") rather than a
// bare program name, and honouring only the first word would run some editors
// in a mode their owner never uses.
func editorCommand() []string {
	for _, name := range []string{"EDITOR", "VISUAL"} {
		if fields := strings.Fields(os.Getenv(name)); len(fields) > 0 {
			return fields
		}
	}
	return []string{fallbackEditor}
}

// reportEdited says what the file just saved cannot say about itself: that it
// can no longer be read at all, that a value in it was refused, or that
// something applied after it decides a setting written there. Each of those is
// otherwise met at the next login, by which time nobody is looking.
func reportEdited(stdout, stderr io.Writer, configDir, path string) int {
	sources := config.LoadSources(configDir)
	for _, source := range sources {
		if source.Path == path && source.Err != nil {
			_, _ = fmt.Fprintf(stderr, "sshakku: %s: %v\n", config.MainFileName, source.Err)
			return 1
		}
	}

	for _, s := range config.Explain(sources, os.LookupEnv) {
		if s.Refused != nil && s.Refused.From.Kind == config.OriginFile && s.Refused.From.Name == path {
			_, _ = fmt.Fprintf(stdout, "%s: %v\n", s.Key, s.Refused.Err)
		}
	}
	for _, o := range config.Overruled(sources, path, os.LookupEnv) {
		_, _ = fmt.Fprintf(stdout, "%s: set here, but %s is what applies\n", o.Key, originName(configDir, o.By))
	}
	return 0
}
