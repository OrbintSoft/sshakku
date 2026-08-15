package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// config reports the configuration in force: every setting, its value, and what
// put that value there. It reads and changes nothing.
func (d deps) config(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	dir := paths.Resolve(paths.FromOS(), paths.ProbeDir).ConfigDir
	switch {
	case len(args) == 1 && args[0] == "--edit":
		return d.configEdit(ctx, stdout, stderr, dir)
	case len(args) > 0:
		_, _ = fmt.Fprintf(stderr, "sshakku: config: unknown argument %q\n\n%s", args[0], usage)
		return 2
	}

	sources := config.LoadSources(dir)
	report := configReport(dir, sources, config.Explain(sources, os.LookupEnv))
	if _, err := io.WriteString(stdout, report); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// configReport renders the report: where the configuration was read from, then
// every setting with the value in force beside what decided it.
func configReport(dir string, sources []config.Source, settings []config.Setting) string {
	var b strings.Builder
	fmt.Fprintf(&b, "config directory: %s\n\nfiles read, in the order they were applied:\n", dir)
	if len(sources) == 0 {
		b.WriteString("  none — no configuration is written here yet\n")
	}
	for _, source := range sources {
		fmt.Fprintf(&b, "  %s", configRelative(dir, source.Path))
		if source.Err != nil {
			fmt.Fprintf(&b, "   (%v)", source.Err)
		}
		b.WriteString("\n")
	}

	b.WriteString("\nsettings:\n")
	for _, s := range settings {
		fmt.Fprintf(&b, "  %-21s %-20s %s\n", s.Key, s.Value, settingSource(dir, s))
	}
	return b.String()
}

// settingSource says what put a setting's value in force, and — for a value
// SSHakku would not use — where the refused one was written, so the reader is
// sent to the file holding it rather than to the file they happen to know
// about.
func settingSource(dir string, s config.Setting) string {
	if s.Refused == nil {
		return originName(dir, s.From)
	}
	return fmt.Sprintf("default, after %s was refused: %v", originName(dir, s.Refused.From), s.Refused.Err)
}

// originName names one origin as the user would go and look for it: a variable
// by the name they exported, a file by its path under the config directory.
func originName(dir string, o config.Origin) string {
	switch o.Kind {
	case config.OriginEnv:
		return "$" + o.Name
	case config.OriginFile:
		return configRelative(dir, o.Name)
	default:
		return "default"
	}
}

// configRelative shortens a path that lies under the config directory, which
// every configuration file does — the directory is named once at the top, and
// repeating it on every line buries the part that differs.
func configRelative(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}
