package launcher

import "context"

// ProcessEntry is one row of a process-table snapshot: a process, the parent it
// names, and the short name the system records for it.
type ProcessEntry struct {
	PID  int
	PPID int
	Name string
}

// SnapshotAncestry answers Parent from a snapshot of the whole process table,
// for a system that has no per-process file to read and reports the tree only
// all at once.
//
// Taking the table is the platform's part and is supplied as Snapshot; reading
// it is here, where it can be checked from any machine. The table is taken once
// and kept: a walk up eight generations that re-read the tree at every step
// could be handed a different tree each time and assemble a chain that never
// existed, quite apart from doing the work eight times.
type SnapshotAncestry struct {
	// Snapshot reports every process the system will admit to, or false if the
	// table could not be taken at all.
	Snapshot func() ([]ProcessEntry, bool)

	taken bool
	table map[int]ProcessEntry
}

// Parent returns the parent pid and short name of pid, or false when the table
// could not be taken or does not hold it.
//
// A process the table does not name is not an error: on a system where a
// process may be invisible to the account asking, the honest answer is that the
// trail stops here, which is what Ancestry does with a false.
func (a *SnapshotAncestry) Parent(ctx context.Context, pid int) (int, string, bool) {
	if ctx.Err() != nil {
		return 0, "", false
	}
	if !a.take() {
		return 0, "", false
	}
	entry, found := a.table[pid]
	if !found {
		return 0, "", false
	}
	return entry.PPID, entry.Name, true
}

// take fills the table on first use and reports whether there is one. A system
// that refused is asked once and not again: the answer stands for as long as
// this source does, and a walk retrying it at every step would spend the cost
// of the refusal eight times over to be told the same thing.
func (a *SnapshotAncestry) take() bool {
	if !a.taken {
		a.taken = true
		if a.Snapshot == nil {
			return false
		}
		entries, ok := a.Snapshot()
		if !ok {
			return false
		}
		a.table = make(map[int]ProcessEntry, len(entries))
		for _, entry := range entries {
			// First writing wins. A table that names one pid twice is a
			// contradiction the reader cannot resolve, and letting the later
			// row overwrite the earlier would make the answer depend on the
			// order the system happened to report them in.
			if _, clash := a.table[entry.PID]; !clash {
				a.table[entry.PID] = entry
			}
		}
	}
	return a.table != nil
}

var _ AncestrySource = (*SnapshotAncestry)(nil)
