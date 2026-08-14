package diagnose

// These cover launcher.ProcfsAncestry.Parent and launcher.ProcfsCgroup.Cgroup branches that are
// reachable on any OS — these types are compiled off Linux too, where they
// degrade to not-ok even though nothing reads /proc there — so the tests are
// deliberately portable (no /proc, no Linux-only test helpers) to keep the
// branches covered on the macOS build as well as Linux.
