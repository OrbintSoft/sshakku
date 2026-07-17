//go:build !linux

package diagnose

// realTmpfsSize has no portable implementation outside Linux yet (Phase 5);
// callers already treat 0 as "could not determine".
func realTmpfsSize(string) int64 { return 0 }
