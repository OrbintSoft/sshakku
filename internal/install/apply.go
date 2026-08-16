package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// Request is what an install or an uninstall was asked to do.
type Request struct {
	// Shell is which interpreter to wire, or Auto to work it out.
	Shell ShellKind
	// Scope is whose sessions it is for.
	Scope Scope
	// Hosts selects between a PowerShell host's two profiles.
	Hosts Hosts
	// ShellExe names the interpreter to ask, instead of finding one.
	ShellExe string
	// Profile names the file to wire outright. It wins over everything else:
	// somebody who names a file has answered the question the rest is for.
	Profile string
	// NoPath skips recording the program's directory in the environment.
	NoPath bool
	// Binary is this program's own path, which is what the hook will run.
	Binary string
}

// Outcome is what was actually done, for the command to report. Nothing here is
// inferred: every field is filled in by the step that did the thing.
type Outcome struct {
	// Shell is the interpreter kind that was wired, once it was worked out.
	Shell ShellKind
	// Interpreter is the program that was asked about itself.
	Interpreter string
	// Wired is the file the wiring went into, which is the one a person goes
	// and looks at.
	Wired string
	// DropIn is true when Wired is a whole file of ours rather than a block
	// inside somebody else's.
	DropIn bool
	// HookFile is the rendered hook the wiring points at.
	HookFile string
	// PathEntry is the directory recorded in the environment, if any.
	PathEntry string
	// PathChanged says whether recording it changed anything.
	PathChanged bool
	// Notes are things the person needs to know that are not failures — a
	// drop-in directory that was there and could not be used, for one.
	Notes []string
}

// Ancestry is how the process tree is read, for working out which shell ran
// this command. It is passed in rather than reached for, so that the
// resolution can be exercised without the machine's own tree.
type Ancestry struct {
	Source launcher.AncestrySource
	Image  ImageSource
	PID    int
}

// Install wires the hook for one shell and returns what it did.
//
// The order is the promise's own: everything that could refuse is asked before
// anything is written. A shell that will never read the hook is reported
// instead of wired, rather than discovered at the next login.
func Install(ctx context.Context, req Request, tree Ancestry) (Outcome, error) {
	p, err := resolve(ctx, req, tree)
	if err != nil {
		return Outcome{}, err
	}

	locations, err := LocationsFor(req.Scope)
	if err != nil {
		return Outcome{}, err
	}

	hookFile, err := renderInto(locations.HookDir, p, req.Binary)
	if err != nil {
		return Outcome{}, err
	}

	sourceLine, err := p.sourceLine(ctx, hookFile)
	if err != nil {
		return Outcome{}, err
	}

	if p.placement.DropIn {
		err = WriteDropIn(p.placement.Path, p.dropInFile(sourceLine))
	} else {
		err = UpsertBlockFile(p.placement.Path, sourceLine)
	}
	if err != nil {
		return Outcome{}, fmt.Errorf("wiring %s: %w", p.placement.Path, err)
	}

	out := Outcome{
		Shell:       p.kind,
		Interpreter: p.interpreter,
		Wired:       p.placement.Path,
		DropIn:      p.placement.DropIn,
		HookFile:    hookFile,
		Notes:       p.notes,
	}
	if !req.NoPath {
		out.PathEntry = filepath.Dir(req.Binary)
		out.PathChanged, err = AddToPath(req.Scope, out.PathEntry)
		if err != nil {
			return out, fmt.Errorf("recording %s in the environment: %w", out.PathEntry, err)
		}
	}
	return out, nil
}

// Uninstall takes the wiring back out, leaving the files as it found them.
//
// Where an install wires one file, an uninstall sweeps every file an install of
// that shell could have chosen: somebody may have installed with one set of
// flags and be uninstalling with another, and a wiring left behind is one that
// goes on running while nothing mentions it any more.
func Uninstall(ctx context.Context, req Request, tree Ancestry) (Outcome, error) {
	p, err := resolve(ctx, req, tree)
	if err != nil {
		return Outcome{}, err
	}

	out := Outcome{Shell: p.kind, Interpreter: p.interpreter, Notes: p.notes}
	for _, candidate := range p.sweep {
		place, err := p.placeIn(ctx, candidate)
		if err != nil {
			return out, err
		}
		if place.DropIn {
			err = RemoveDropIn(place.Path)
		} else {
			err = StripBlockFile(place.Path)
		}
		if err != nil {
			return out, fmt.Errorf("unwiring %s: %w", place.Path, err)
		}
		if place.Path == p.placement.Path {
			out.Wired = place.Path
			out.DropIn = place.DropIn
		}
	}

	locations, err := LocationsFor(req.Scope)
	if err != nil {
		return out, err
	}
	out.HookFile = filepath.Join(locations.HookDir, p.hookName())
	if err := os.Remove(out.HookFile); err != nil && !os.IsNotExist(err) {
		return out, fmt.Errorf("removing %s: %w", out.HookFile, err)
	}

	if !req.NoPath {
		// The entry is taken out; the recorded previous value is not put back.
		// Restoring it would discard everything changed since installing, where
		// what was promised is that this entry goes and every other one stays.
		out.PathEntry = filepath.Dir(req.Binary)
		out.PathChanged, err = RemoveFromPath(req.Scope, out.PathEntry)
		if err != nil {
			return out, fmt.Errorf("taking %s back out of the environment: %w", out.PathEntry, err)
		}
	}
	return out, nil
}

// plan is everything worked out before anything is written.
type plan struct {
	kind        ShellKind
	interpreter string
	// dialect is the language the wiring is written in, settled once so that
	// nothing below has to ask again or has anywhere to disagree.
	dialect shell.Dialect
	// spelling moves paths between this program's way of writing one and the
	// shell's, where those differ.
	spelling spelling
	// placement is where the wiring goes for the flags given.
	placement Placement
	// sweep is every file an install of this shell could have wired, which is
	// what an uninstall has to look in.
	sweep []string
	notes []string
}

// resolve works out which shell, which file, and whether that file will ever be
// read — before anything is written.
func resolve(ctx context.Context, req Request, tree Ancestry) (plan, error) {
	kind, interpreter, err := whichShell(ctx, req, tree)
	if err != nil {
		return plan{}, err
	}

	p := plan{kind: kind, interpreter: interpreter, spelling: spellingFor(interpreter)}
	if p.dialect, err = shell.Named(dialectName(kind)); err != nil {
		return plan{}, err
	}

	switch kind {
	case PowerShellCore, WindowsPowerShell:
		err = p.forPowerShell(ctx, req)
	case Bash, Zsh:
		err = p.forBourne(ctx, req)
	case Auto:
		// whichShell settles Auto into a real kind or fails; arriving here
		// would mean it had returned the question instead of an answer.
		err = fmt.Errorf("the shell was not worked out, so there is nothing to wire")
	default:
		err = fmt.Errorf("there is no wiring defined for a %s shell", kind)
	}
	if err != nil {
		return plan{}, err
	}
	return p, nil
}

// whichShell settles the interpreter, by what was asked for or by looking.
//
// The three ways of asking are in the order of how much they say. A program
// named outright is used and merely checked; a kind named is looked for; and
// nothing named is the ancestry of this very process, which is what makes
// `sshakku install`, typed in a window, wire that window's shell.
func whichShell(ctx context.Context, req Request, tree Ancestry) (ShellKind, string, error) {
	if req.ShellExe != "" {
		kind, ok := RecogniseShell(req.ShellExe)
		if !ok {
			return "", "", fmt.Errorf("%s is not a shell this system can wire", req.ShellExe)
		}
		if req.Shell != Auto && req.Shell != kind {
			return "", "", fmt.Errorf("%s is a %s, but --shell says %s", req.ShellExe, kind, req.Shell)
		}
		return kind, req.ShellExe, nil
	}
	if req.Shell != Auto && req.Shell != "" {
		exe, err := lookInterpreter(ctx, req.Shell)
		if err != nil {
			return "", "", err
		}
		return req.Shell, exe, nil
	}
	return ResolveShell(ctx, tree.PID, tree.Source, tree.Image)
}

// forPowerShell settles a PowerShell target, asking the interpreter about
// itself.
//
// The asking and the deciding are separate because the paths are the
// interpreter's own answer and nothing here may assume them, while what is done
// with that answer is a decision that has to be checkable against an
// interpreter this machine does not have — a Store installation, a redirected
// Documents folder, a locked-down host.
func (p *plan) forPowerShell(ctx context.Context, req Request) error {
	host, err := AskHost(ctx, p.interpreter)
	if err != nil {
		return err
	}
	return p.forHost(ctx, host, req)
}

// forHost settles the target from what one PowerShell said about itself, and
// refuses a profile that host will not run.
func (p *plan) forHost(ctx context.Context, host Host, req Request) error {
	// Asked before anything is written. A profile that will not be run is not
	// a wiring that half worked; it is one that would never have run at all,
	// and the person has to hear that now rather than at their next login.
	if note, willNot := willNotRunItsProfile(host); willNot {
		return fmt.Errorf("%s", note)
	}

	var err error
	target := req.Profile
	if target == "" {
		p.sweep = EveryProfile(host)
		if target, err = ProfileFor(host, req.Scope, req.Hosts); err != nil {
			return err
		}
	} else {
		// Somebody who named a file named the whole of what to touch. Sweeping
		// the others would remove a wiring they did not ask about.
		p.sweep = []string{target}
	}

	if p.placement, err = p.placeIn(ctx, target); err != nil {
		return err
	}
	p.note(p.placement.Why)
	return nil
}

// forBourne settles a Bourne target, asking the shell where it will look.
func (p *plan) forBourne(ctx context.Context, req Request) error {
	if req.Profile != "" {
		p.sweep = []string{req.Profile}
		place, err := p.placeIn(ctx, req.Profile)
		if err != nil {
			return err
		}
		p.placement = place
		p.note(place.Why)
		return nil
	}

	answered, err := AskShell(ctx, p.interpreter)
	if err != nil {
		return err
	}
	login, err := BourneLoginFile(answered.Home, func(path string) bool { return p.exists(ctx, path) })
	if err != nil {
		return err
	}

	// Every file a login shell might have been wired into, whichever of them
	// existed at the time: the choice above depends on what was there, so an
	// uninstall cannot reach the same answer by making it again.
	for _, name := range loginFiles {
		candidate, err := under(answered.Home, name)
		if err != nil {
			return err
		}
		p.sweep = append(p.sweep, candidate)
	}

	if p.placement, err = p.placeIn(ctx, login); err != nil {
		return err
	}
	p.note(p.placement.Why)
	return nil
}

// placeIn decides where the wiring goes beside one startup file, having first
// put that file into this program's own spelling — the shell named it, and on
// the system where the two differ nothing here could open it as it stands.
func (p plan) placeIn(ctx context.Context, file string) (Placement, error) {
	ours, err := p.spelling.forUs(ctx, file)
	if err != nil {
		return Placement{}, err
	}
	if p.bourne() {
		return PlaceBourne(ours, p.dropInName())
	}
	return PlacePowerShell(ours, p.dropInName())
}

// sourceLine is what goes into the startup file: the one line that runs the
// rendered hook, with the path written the way the shell being wired reads one
// and quoted the way that shell's language reads a literal.
func (p plan) sourceLine(ctx context.Context, hookFile string) (string, error) {
	spelt, err := p.spelling.forShell(ctx, hookFile)
	if err != nil {
		return "", err
	}
	return ". " + p.dialect.Quote(spelt), nil
}

// bourne reports whether the shell being wired reads Bourne syntax, which is
// what decides every remaining difference between the two kinds of target: the
// language of the hook, of the drop-in, and the names both are given.
func (p plan) bourne() bool {
	return p.kind == Bash || p.kind == Zsh
}

// template is the hook to render and the literal in it that holds the path.
func (p plan) template() ([]byte, string) {
	if p.bourne() {
		return bourneHookTemplate, BourneBinaryPlaceholder
	}
	return powerShellHookTemplate, PowerShellBinaryPlaceholder
}

// dropInFile is the whole content of the wrapper that goes into a drop-in
// directory, in the language of the shell that reads that directory.
func (p plan) dropInFile(sourceLine string) []byte {
	if p.bourne() {
		return BourneDropIn(sourceLine)
	}
	return []byte("# sshakku shell hook. Regenerate by re-running the sshakku install.\n" + sourceLine + "\n")
}

func (p plan) dropInName() string {
	if p.bourne() {
		return "50-sshakku-init.sh"
	}
	return "50-sshakku-init.ps1"
}

func (p plan) hookName() string {
	if p.bourne() {
		return "shell-hook.sh"
	}
	return "shell-hook.ps1"
}

// exists reports whether a path the shell named is there, looked at in this
// program's own spelling. A path that cannot be translated is reported absent:
// what the caller does with the answer is choose a file to create, and a name
// this program cannot resolve is not one it can say it found.
func (p plan) exists(ctx context.Context, path string) bool {
	ours, err := p.spelling.forUs(ctx, path)
	if err != nil {
		return false
	}
	_, err = os.Stat(ours)
	return err == nil
}

func (p *plan) note(text string) {
	if strings.TrimSpace(text) != "" {
		p.notes = append(p.notes, text)
	}
}

// willNotRunItsProfile reports a host that cannot run the file an install would
// write into, and what it would take to change that.
func willNotRunItsProfile(host Host) (string, bool) {
	switch host.EffectiveExecutionPolicy {
	case "Restricted", "AllSigned":
		return fmt.Sprintf("this PowerShell will not run its profile: its execution policy is %s,"+
			" so nothing written there would ever run. Change it with"+
			" `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser` and install again,"+
			" or wire a different shell", host.EffectiveExecutionPolicy), true
	}
	if host.LanguageMode != "" && host.LanguageMode != "FullLanguage" {
		return fmt.Sprintf("this PowerShell is running in %s, in which a profile of this kind cannot run,"+
			" so nothing written there would take effect", host.LanguageMode), true
	}
	return "", false
}

// dialectName is the language one kind of shell reads.
func dialectName(kind ShellKind) string {
	if kind == PowerShellCore || kind == WindowsPowerShell {
		return shell.PowerShell
	}
	return shell.Posix
}
