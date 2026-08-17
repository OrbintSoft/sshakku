package install

import (
	"context"
	"path/filepath"
)

// machineWiring is where a machine-wide wiring goes for one Bourne shell: a
// directory every login shell of the system reads one file at a time, or a
// single startup file every login shell reads. Which of the two a system offers
// is that system's own answer; what is done with it is here, so both answers
// stay checkable from either machine.
//
// The paths are in the shell's own spelling, which under a POSIX-emulating
// environment is not this program's — they are translated before anything opens
// one, the same as every other path a shell names.
type machineWiring struct {
	// DropInDir is a directory every login shell reads. It is this install's to
	// create, unlike the directory beside an account's own startup file: that
	// one is read because somebody's profile loops over it, and its existence is
	// the evidence of that, while this one is read by the shell itself whether
	// or not anybody has made it yet.
	DropInDir string
	// File is the startup file to put a marked block in, for a system whose
	// machine-wide startup has no such directory.
	File string
}

// forMachine settles a machine-wide Bourne target from what this system offers.
//
// There is no asking the shell where it looks here, because the answer is not
// the shell's to give: a machine-wide startup file belongs to the system and is
// the same one whichever account is running the install. That is the whole
// difference between the two scopes, and it is why the account's own home
// directory must not appear anywhere in what this settles.
func (p *plan) forMachine(ctx context.Context, target machineWiring) error {
	if target.DropInDir != "" {
		dir, err := p.spelling.forUs(ctx, target.DropInDir)
		if err != nil {
			return err
		}
		p.dropInDir = dir
		p.placement = Placement{Path: filepath.Join(dir, p.dropInName()), DropIn: true}
		p.sweep = []Placement{p.placement}
		return nil
	}

	// A marked block inside the system's own file, and not the drop-in rule the
	// per-account targets use: a `.d` directory beside a machine-wide startup
	// file is not a convention any of these systems reads by itself, so putting
	// a hook in one would be wiring a file nothing runs.
	file, err := p.spelling.forUs(ctx, target.File)
	if err != nil {
		return err
	}
	p.placement = Placement{Path: file}
	p.sweep = []Placement{p.placement}
	return nil
}
