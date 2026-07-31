package keepassxc

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// socketName is the endpoint KeePassXC listens on. The name is KeePassXC's.
const socketName = "org.keepassxc.KeePassXC.BrowserServer"

// ErrNotRunning reports that no KeePassXC socket answered. It means the app is
// not running, or its browser integration is switched off — a condition the
// user can fix, and the one that sends the caller to the fallback route.
var ErrNotRunning = errors.New("keepassxc: no running KeePassXC with browser integration enabled was found")

// SocketPaths returns the candidate socket paths, most specific first.
func SocketPaths() []string {
	return socketPaths(runtime.GOOS, os.TempDir(), os.Getenv("XDG_RUNTIME_DIR"))
}

// socketPaths is SocketPaths with the platform and the environment passed in,
// so the choice each OS makes can be checked from any OS.
//
// Where KeePassXC puts the socket differs by platform and by version, and it
// offers no way to ask. Linux has used a per-application subdirectory of
// XDG_RUNTIME_DIR since KeePassXC 2.7 and the runtime directory itself before
// that; macOS uses the per-user temporary directory. Both fall back to /tmp.
// Returning the candidates rather than one guess lets the caller report which
// ones it tried when none answers, which is the difference between a
// diagnosable failure and "it does not work".
func socketPaths(goos, tempDir, runtimeDir string) []string {
	var dirs []string
	switch {
	case goos == "darwin":
		dirs = append(dirs, tempDir)
	case runtimeDir != "":
		dirs = append(dirs,
			filepath.Join(runtimeDir, "app", "org.keepassxc.KeePassXC"),
			runtimeDir,
		)
	}
	dirs = append(dirs, "/tmp")

	paths := make([]string, 0, len(dirs))
	seen := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		path := filepath.Join(dir, socketName)
		if seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}
