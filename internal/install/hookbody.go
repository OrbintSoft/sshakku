package install

import (
	"bytes"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
)

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
		return nil, fmt.Errorf("no binary was named to write into the hook")
	}
	if !bytes.Contains(template, []byte(placeholder)) {
		// A template that has lost its placeholder renders a hook naming some
		// other binary, or none. It would be written out, wired up, and would
		// quietly do nothing or the wrong thing at every login.
		return nil, fmt.Errorf("this hook template does not contain %s, so there is nowhere to write the"+
			" path of the binary into it", placeholder)
	}
	return bytes.ReplaceAll(template, []byte(placeholder), []byte(dialect.Quote(binary))), nil
}
