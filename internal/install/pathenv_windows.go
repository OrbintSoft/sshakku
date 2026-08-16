//go:build windows

package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// pathValue is the name of the value holding the search list, under both of the
// keys below.
const pathValue = "Path"

// environmentLocation is where one scope's environment variables are kept.
//
// It is a value rather than a constant reached for inside the functions below,
// so that a test can be given a key of its own. Nothing that writes here may be
// exercised against the real one: a test that damaged an account's search list
// would damage the machine it ran on, permanently and outside the directory it
// was given to play in.
type environmentLocation struct {
	root registry.Key
	path string
}

// environmentFor is where this system keeps a scope's environment.
//
// The per-account key is the environment itself: every value under it is a
// variable of that account. That is worth knowing before putting anything there
// which is not meant to become one.
func environmentFor(scope Scope) (environmentLocation, error) {
	switch scope {
	case User:
		return environmentLocation{root: registry.CURRENT_USER, path: `Environment`}, nil
	case Machine:
		return environmentLocation{
			root: registry.LOCAL_MACHINE,
			path: `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		}, nil
	default:
		return environmentLocation{}, unknownScope(scope)
	}
}

// readPath returns the stored search list exactly as it is stored, with the
// kind it is stored as.
//
// Exactly as stored is the whole point. This value habitually refers to other
// variables, and it is kept as REG_EXPAND_SZ so that it means whatever they
// mean when a session reads it. Reading it through anything that resolves them
// and writing that back would replace every reference with what it happened to
// mean during the install — the account's search list would then be wrong the
// first time anything on the machine moved, and nothing would say why.
//
// A value that is not there is an empty list rather than an error: an account
// need not have a search list of its own until something gives it one.
func readPath(where environmentLocation) (string, uint32, error) {
	key, err := registry.OpenKey(where.root, where.path, registry.QUERY_VALUE)
	if err != nil {
		return "", 0, fmt.Errorf("opening %s to read the search list: %w", where.path, err)
	}
	defer func() { _ = key.Close() }()

	raw, kind, err := key.GetStringValue(pathValue)
	if errors.Is(err, registry.ErrNotExist) {
		// Stored as the kind that may refer to other variables, which is what
		// this system stores it as and what anything written here later will
		// want it to be.
		return "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("reading the search list from %s: %w", where.path, err)
	}
	return raw, kind, nil
}

// writePath stores the search list back under the kind it was found as.
//
// A kind this does not understand is refused rather than converted. Rewriting a
// value as a kind it was not is a change to what every session gets, made by
// something that was asked only to add a directory.
func writePath(where environmentLocation, raw string, kind uint32) error {
	key, err := registry.OpenKey(where.root, where.path, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening %s to write the search list: %w", where.path, err)
	}
	defer func() { _ = key.Close() }()

	switch kind {
	case registry.EXPAND_SZ:
		err = key.SetExpandStringValue(pathValue, raw)
	case registry.SZ:
		err = key.SetStringValue(pathValue, raw)
	default:
		return fmt.Errorf("the search list in %s is stored as kind %d, which this does not know how to write back"+
			" without changing what it is; leaving it alone", where.path, kind)
	}
	if err != nil {
		return fmt.Errorf("writing the search list to %s: %w", where.path, err)
	}
	return nil
}

// pathBackup is the previous search list, kept so it can be put back.
type pathBackup struct {
	// Value is the list exactly as it was stored, unresolved.
	Value string `json:"value"`
	// Kind is how it was stored, so putting it back restores that too.
	Kind uint32 `json:"kind"`
	// Scope says which environment it came out of, so a file found on its own
	// says what it is.
	Scope Scope `json:"scope"`
}

// backupFile is where a scope's previous search list is kept.
//
// A file, and deliberately not a value beside the one being changed: every
// value under the per-account environment key is a variable of that account, so
// a backup put there would become one — a stray variable in every session, made
// by the step that exists to avoid exactly that kind of damage.
func backupFile(scope Scope) (string, error) {
	where, err := LocationsFor(scope)
	if err != nil {
		return "", err
	}
	return filepath.Join(where.HookDir, fmt.Sprintf("path-before-sshakku-%s.json", scope)), nil
}

// keepPreviousPath writes down what the search list was, before it is changed.
//
// It is written on every change and not only the first, and that is right
// because a change only happens when there is one to make: adding a directory
// already there changes nothing and so records nothing, and the file goes on
// holding the list as it was before this program first touched it.
func keepPreviousPath(scope Scope, raw string, kind uint32) error {
	path, err := backupFile(scope)
	if err != nil {
		return err
	}
	content, err := json.MarshalIndent(pathBackup{Value: raw, Kind: kind, Scope: scope}, "", "  ")
	if err != nil {
		return fmt.Errorf("recording the previous search list: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("making somewhere to record the previous search list: %w", err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o644); err != nil {
		return fmt.Errorf("recording the previous search list in %s: %w", path, err)
	}
	return nil
}

// changePath applies change to the stored search list, recording what was there
// first and telling the system afterwards. It reports whether anything changed.
func changePath(where environmentLocation, scope Scope, change func(string) (string, bool)) (bool, error) {
	raw, kind, err := readPath(where)
	if err != nil {
		return false, err
	}

	updated, changed := change(raw)
	if !changed {
		return false, nil
	}

	if err := keepPreviousPath(scope, raw, kind); err != nil {
		return false, err
	}
	if err := writePath(where, updated, kind); err != nil {
		return false, err
	}
	announceEnvironmentChange()
	return true, nil
}

// AddToPath records dir in the search list of the given scope, so that sessions
// started later find the program without being told where it is. It reports
// whether anything changed; running an install twice changes it once.
func AddToPath(scope Scope, dir string) (bool, error) {
	where, err := environmentFor(scope)
	if err != nil {
		return false, err
	}
	list := PersistentPathList()
	return changePath(where, scope, func(raw string) (string, bool) { return list.Add(raw, dir) })
}

// RemoveFromPath takes dir back out of the search list of the given scope,
// leaving every other entry as it was.
func RemoveFromPath(scope Scope, dir string) (bool, error) {
	where, err := environmentFor(scope)
	if err != nil {
		return false, err
	}
	list := PersistentPathList()
	return changePath(where, scope, func(raw string) (string, bool) { return list.Remove(raw, dir) })
}

// announceEnvironmentChange tells the programs already running that the stored
// environment has changed.
//
// Without it the new entry reaches only sessions started by something that read
// the environment afresh, which in practice means after a logout: the desktop
// shell hands every program it starts the environment it read when it started,
// and it started before this ran. The message is sent with a bound and its
// result ignored on purpose — a program that is wedged must not wedge an
// install, and every one that does not answer simply keeps the environment it
// had, which is the situation this is trying to improve rather than one it can
// make worse.
func announceEnvironmentChange() {
	const (
		hwndBroadcast   = 0xFFFF
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
		timeoutMs       = 1000
	)

	environment, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	_, _, _ = sendMessageTimeout.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(environment)),
		uintptr(smtoAbortIfHung),
		uintptr(timeoutMs),
		uintptr(unsafe.Pointer(&result)),
	)
}

var sendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
