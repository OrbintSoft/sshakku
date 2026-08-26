package install

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
)

// The hook templates, carried inside the binary so that an install needs
// nothing beside it. Each is a real file rather than a string in this source,
// so that it can be read, diffed and linted as the language it is written in —
// and, for the Bourne one, rendered by the shell installer as well, from the
// same bytes.
//
// The byte-order mark on the PowerShell template is deliberate and travels into
// the rendered hook: Windows PowerShell reads a script file without one in the
// machine's ANSI code page, which corrupts exactly the non-ASCII paths a hook
// is most likely to carry.
var (
	//go:embed nn-sshakku-init.ps1
	powerShellHookTemplate []byte
	//go:embed nn-ssh-init.sh
	bourneHookTemplate []byte
)

// hookMode is what a rendered hook is given. It matches what the shell
// installer gives the same file, because a machine can be wired by either.
const hookMode fs.FileMode = 0o755

// hookDirMode is what the directory holding the rendered hooks is given when
// this program has to create it. It is not a secret and every session of the
// scope has to read it; only the account or the administrator that installed
// may write.
const hookDirMode fs.FileMode = 0o755

// The literal each hook template carries where the path of the binary belongs.
//
// What is replaced is the whole literal, quotes and all, and not the text
// inside it. That keeps each template a valid file of its own language — one
// that its own linter reads, and that a person can open and understand — while
// leaving the quoting entirely to the renderer. A placeholder inside the quotes
// would mean a path was quoted half by the template and half by whatever put
// the value there, which is how an apostrophe ends up ending a string it was
// supposed to be inside.
const (
	// PowerShellBinaryPlaceholder is what nn-sshakku-init.ps1 carries.
	PowerShellBinaryPlaceholder = `'@SSHAKKU_BIN@'`
	// BourneBinaryPlaceholder is what nn-ssh-init.sh carries. It is a plausible
	// path rather than a token because that file is also rendered by the shell
	// installer, which replaces it with sed and has no token to look for.
	BourneBinaryPlaceholder = `"/usr/local/bin/sshakku"`
)

// errNoBinaryNamed is a render asked to write no path into the hook. A hook
// naming nothing is one that runs nothing at every login.
var errNoBinaryNamed = errors.New("no binary was named to write into the hook")

// templateWithoutPlaceholderError is a hook template that has lost the token
// the binary's path goes into. Rendering it would produce a hook naming some
// other binary, or none, which would be written out and wired up all the same.
type templateWithoutPlaceholderError struct{ placeholder string }

func (e templateWithoutPlaceholderError) Error() string {
	return fmt.Sprintf("this hook template does not contain %s, so there is nowhere to write the"+
		" path of the binary into it", e.placeholder)
}

// RenderHook returns the hook template with the path of the binary written into
// it, as a literal the template's own language reads back unchanged.
//
// The quoting is the point. A path on the system this is for goes through an
// account's own directory, and an account may be called O'Brien: an apostrophe
// dropped unescaped into a PowerShell literal ends it, and everything after it
// — the rest of the path, and then the rest of the line — is read as code. The
// dialect's own quoting is used rather than a rule invented here, so a hook is
// quoted the same way as every other line this program prints for that shell.
func RenderHook(template []byte, placeholder, binary string, dialect shell.Dialect) ([]byte, error) {
	if binary == "" {
		return nil, errNoBinaryNamed
	}
	if !bytes.Contains(template, []byte(placeholder)) {
		// A template that has lost its placeholder renders a hook naming some
		// other binary, or none. It would be written out, wired up, and would
		// quietly do nothing or the wrong thing at every login.
		return nil, templateWithoutPlaceholderError{placeholder: placeholder}
	}
	return bytes.ReplaceAll(template, []byte(placeholder), []byte(dialect.Quote(binary))), nil
}

// renderInto writes the rendered hook into hookDir and returns where it put it.
//
// The directory is created if it is not there, which is the ordinary case on a
// first install. The file goes down through the same atomic replace every other
// startup file here does: a session logging in at that moment reads the whole
// of the old hook or the whole of the new one, never half of each.
func renderInto(hookDir string, p plan, binary string) (string, error) {
	template, placeholder := p.template()
	rendered, err := RenderHook(template, placeholder, binary, p.dialect)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(hookDir, hookDirMode); err != nil {
		return "", fmt.Errorf("making the directory for the hook, %s: %w", hookDir, err)
	}
	path := filepath.Join(hookDir, p.hookName())
	if err := replace(path, rendered, hookMode); err != nil {
		return "", err
	}
	return path, nil
}
