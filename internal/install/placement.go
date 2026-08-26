package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Placement is where one shell's hook goes: a file of our own inside a
// directory the shell reads, or a marker block inside the shell's own startup
// file.
type Placement struct {
	// Path is the file to write.
	Path string
	// DropIn is true when Path is a whole file of ours, false when it is
	// somebody else's file a block goes into.
	DropIn bool
	// Why records a drop-in directory that was there and was not used, so the
	// install can say why it did the other thing. Empty when there is nothing
	// to explain.
	Why string
}

// BourneDropInDir is the directory a Bourne shell's drop-in would go in:
// `<startup file>.d`, beside the file itself.
func BourneDropInDir(startupFile string) string {
	return startupFile + ".d"
}

// PowerShellDropInDir is the directory a PowerShell drop-in would go in:
// `Profile.d`, beside the profile.
func PowerShellDropInDir(profile string) string {
	return filepath.Join(filepath.Dir(profile), "Profile.d")
}

// The three refusals a placement can meet. The first two are whole sentences:
// there is nothing to name, so there is nothing to interpolate. The third holds
// the invariant half, and the path keeps the front of the line.
var (
	errNoStartupFileNamed = errors.New("no startup file was named, so there is nowhere to put the hook")
	errNoProfileNamed     = errors.New("no profile was named, so there is nowhere to put the hook")
	errNotADirectory      = errors.New("is there but is not a directory")
)

// PlaceBourne decides where a Bourne shell's hook goes.
//
// A drop-in directory beside the startup file is used when it is there. Its
// existence is the whole condition: these directories are a convention of the
// shell's own configuration, and one that exists is one somebody's profile
// loops over. Nothing here creates it, because creating it would be creating a
// directory nothing reads.
func PlaceBourne(startupFile, dropInName string) (Placement, error) {
	if startupFile == "" {
		return Placement{}, errNoStartupFileNamed
	}

	dir := BourneDropInDir(startupFile)
	there, err := isDir(dir)
	if err != nil {
		return Placement{}, err
	}
	if there {
		return Placement{Path: filepath.Join(dir, dropInName), DropIn: true}, nil
	}
	return Placement{Path: startupFile}, nil
}

// PlacePowerShell decides where a PowerShell hook goes.
//
// The condition is not the one above, and the difference is the whole reason
// this is a separate function. PowerShell reads no drop-in directory of its
// own: nothing but the profile itself can load one, so a directory that exists
// says only that somebody made a directory. Dropping a file into one that no
// profile loops over would install a hook that never runs, and report success.
// So the profile is read, and the directory is used only when the profile
// refers to it.
//
// What the profile is asked about is the user's own content. An earlier install
// of ours is stripped out first: a block that mentioned the directory would
// otherwise be taken as the code that reads it, and this would talk itself into
// a drop-in on the strength of something it wrote.
func PlacePowerShell(profile, dropInName string) (Placement, error) {
	if profile == "" {
		return Placement{}, errNoProfileNamed
	}

	dir := PowerShellDropInDir(profile)
	there, err := isDir(dir)
	if err != nil {
		return Placement{}, err
	}
	if !there {
		return Placement{Path: profile}, nil
	}

	read, err := profileReadsDropIns(profile, filepath.Base(dir))
	if err != nil {
		return Placement{}, err
	}
	if read {
		return Placement{Path: filepath.Join(dir, dropInName), DropIn: true}, nil
	}
	return Placement{
		Path: profile,
		Why: fmt.Sprintf("%s is there but nothing in %s loads it, and PowerShell does not load such a"+
			" directory by itself, so the hook went into the profile instead", dir, profile),
	}, nil
}

// profileReadsDropIns reports whether the profile's own content refers to the
// drop-in directory by name.
//
// A profile that is not there yet reads nothing, which is not an error: an
// install writes one where there was none.
func profileReadsDropIns(profile, dirName string) (bool, error) {
	content, err := os.ReadFile(profile)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s to see whether it loads its drop-in directory: %w", profile, err)
	}
	return strings.Contains(string(StripBlock(content)), dirName), nil
}

// isDir reports whether path is a directory. Something there that is not a
// directory is neither a drop-in directory nor nothing: reporting it as absent
// would put the hook in the profile and leave the real problem unmentioned.
func isDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("looking for the drop-in directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s %w", path, errNotADirectory)
	}
	return true, nil
}
