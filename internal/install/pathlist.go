package install

import "strings"

// PathList describes how one system spells a list of directories held in a
// single environment variable, and knows how to add one entry to such a list
// and take it out again without disturbing anything else in it.
//
// The list is handled as text and never as a set of paths. On the system this
// matters for, the stored value routinely refers to other variables — a `PATH`
// mentioning `%SystemRoot%` means whatever that is at the moment it is read,
// which is the point of writing it that way. Anything here that resolved,
// cleaned or reordered entries would flatten those into what they happened to
// mean during the install, and every one of them would be wrong the first time
// the machine changed.
type PathList struct {
	// Separator stands between two entries.
	Separator string
	// SameEntry decides whether two entries name the same directory. It is a
	// system's own rule: whether case matters, and what a trailing separator
	// means, are not the same answer everywhere.
	SameEntry func(a, b string) bool
}

// Add returns the list with dir in it, and whether that changed anything.
//
// The entry goes at the end. A persistent list is read by every session the
// account or the machine starts from then on, and putting an entry at the front
// of one changes which program every other name on it resolves to — a cost far
// out of proportion to being found slightly sooner.
//
// Adding an entry already there changes nothing, however many times an install
// is run. Entries are compared as they are written: an install always writes
// the same literal, so it always recognises its own. A hand-written entry that
// names the same directory through a variable is not recognised as the same one
// and would be joined rather than replaced, which is the honest outcome — this
// cannot know what that variable will mean later without resolving it, and
// resolving it is the one thing it must not do.
func (l PathList) Add(raw, dir string) (string, bool) {
	if dir == "" {
		return raw, false
	}
	if l.has(raw, dir) {
		return raw, false
	}
	if strings.TrimSpace(raw) == "" {
		return dir, true
	}
	// A list already ending in a separator gets the entry, not a second
	// separator: an empty entry between them would be one more directory
	// searched, and on this system an empty entry is the current one.
	if strings.HasSuffix(raw, l.Separator) {
		return raw + dir, true
	}
	return raw + l.Separator + dir, true
}

// Remove returns the list without any entry naming dir, and whether that
// changed anything.
//
// Every other entry survives exactly as it was written, including ones that are
// empty and ones that refer to variables. What is removed is removed with its
// separator, so a list does not gain an empty entry where ours used to be.
func (l PathList) Remove(raw, dir string) (string, bool) {
	if dir == "" || raw == "" {
		return raw, false
	}

	entries := strings.Split(raw, l.Separator)
	kept := make([]string, 0, len(entries))
	for _, entry := range entries {
		if l.same(entry, dir) {
			continue
		}
		kept = append(kept, entry)
	}
	if len(kept) == len(entries) {
		return raw, false
	}
	return strings.Join(kept, l.Separator), true
}

// has reports whether the list already names dir.
func (l PathList) has(raw, dir string) bool {
	for _, entry := range strings.Split(raw, l.Separator) {
		if l.same(entry, dir) {
			return true
		}
	}
	return false
}

// same applies the system's own comparison. It is never reached with an empty
// dir: Add and Remove refuse that before asking, because an empty entry is a
// real entry with a meaning of its own and is nobody's to add or take away.
func (l PathList) same(entry, dir string) bool {
	if l.SameEntry == nil {
		return entry == dir
	}
	return l.SameEntry(entry, dir)
}

// trimTrailingSeparators removes the directory separators at the end of a path,
// which name the same directory as the path without them. It leaves a path that
// is nothing but separators alone, since that is a root and not an empty name.
func trimTrailingSeparators(path string, separators string) string {
	trimmed := strings.TrimRight(path, separators)
	if trimmed == "" {
		return path
	}
	return trimmed
}
