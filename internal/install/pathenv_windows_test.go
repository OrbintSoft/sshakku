//go:build windows

package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

// scratch makes a key of this test's own and returns where it is.
//
// Nothing here is ever pointed at the real environment. A test that wrote there
// would damage the account it ran under, permanently and well outside the
// directory it was lent — and it would do it on a developer's own machine, not
// on a runner that gets thrown away.
func scratch(t *testing.T) environmentLocation {
	t.Helper()

	where := environmentLocation{
		root: registry.CURRENT_USER,
		path: fmt.Sprintf(`Software\SSHakku\test-%s-%d`, t.Name(), os.Getpid()),
	}
	key, _, err := registry.CreateKey(where.root, where.path, registry.ALL_ACCESS)
	require.NoError(t, err)
	require.NoError(t, key.Close())
	t.Cleanup(func() { _ = registry.DeleteKey(where.root, where.path) })

	real, err := environmentFor(User)
	require.NoError(t, err)
	require.NotEqual(t, real.path, where.path, "this must never be the account's own environment")
	return where
}

// backupIn points the recorded previous value at a directory of the test's own,
// since keeping it is part of what changing the list does.
func backupIn(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	return dir
}

// The value refers to other variables on purpose, and is stored as the kind
// that means "resolve these when read". Read through anything that resolved
// them and written back, the account's search list would be frozen into what
// those variables happened to mean during the install.
func TestTheStoredListIsReadAndWrittenExactlyAsItIsStored(t *testing.T) {
	where := scratch(t)
	original := `%SystemRoot%\system32;%USERPROFILE%\bin;C:\Program Files\Git\cmd`
	require.NoError(t, writePath(where, original, registry.EXPAND_SZ))

	raw, kind, err := readPath(where)

	require.NoError(t, err)
	assert.Equal(t, original, raw, "every reference has to come back a reference, not what it currently means")
	assert.Equal(t, uint32(registry.EXPAND_SZ), kind)
	assert.Contains(t, raw, "%SystemRoot%")
	assert.NotContains(t, raw, `C:\Windows\system32`, "which is what resolving it would have produced")
}

func TestTheKindItWasStoredAsIsTheKindItIsWrittenBackAs(t *testing.T) {
	for _, kind := range []uint32{registry.EXPAND_SZ, registry.SZ} {
		where := scratch(t)
		require.NoError(t, writePath(where, `C:\one`, kind))

		list := PersistentPathList()
		_, err := changePath(where, User, func(raw string) (string, bool) { return list.Add(raw, `C:\two`) })
		require.NoError(t, err)

		_, found, err := readPath(where)
		require.NoError(t, err)
		assert.Equal(t, kind, found, "a value rewritten as another kind is a change nobody asked for")
	}
}

func TestAKindThisDoesNotUnderstandIsLeftAlone(t *testing.T) {
	where := scratch(t)

	err := writePath(where, "anything", registry.MULTI_SZ)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "leaving it alone")
}

// An account need not have a search list of its own until something gives it
// one, so nothing there is an empty list and not a failure.
func TestAnAccountWithNoListOfItsOwnHasAnEmptyOne(t *testing.T) {
	where := scratch(t)

	raw, kind, err := readPath(where)

	require.NoError(t, err)
	assert.Empty(t, raw)
	assert.Equal(t, uint32(registry.EXPAND_SZ), kind,
		"and the kind it will be created as is the one that may refer to other variables")
}

// The whole round trip against a real key: install, install again, uninstall.
func TestAddingIsIdempotentAndRemovingGivesBackWhatWasThere(t *testing.T) {
	where := scratch(t)
	backupIn(t)
	original := `%SystemRoot%\system32;;C:\Program Files\Git\cmd`
	require.NoError(t, writePath(where, original, registry.EXPAND_SZ))
	ours := `C:\Users\example\AppData\Local\Programs\sshakku`
	list := PersistentPathList()

	changed, err := changePath(where, User, func(raw string) (string, bool) { return list.Add(raw, ours) })
	require.NoError(t, err)
	assert.True(t, changed)

	after, _, err := readPath(where)
	require.NoError(t, err)
	assert.Equal(t, original+";"+ours, after)

	changed, err = changePath(where, User, func(raw string) (string, bool) { return list.Add(raw, ours) })
	require.NoError(t, err)
	assert.False(t, changed, "however many times an install is run, the entry is there once")

	changed, err = changePath(where, User, func(raw string) (string, bool) { return list.Remove(raw, ours) })
	require.NoError(t, err)
	assert.True(t, changed)

	back, kind, err := readPath(where)
	require.NoError(t, err)
	assert.Equal(t, original, back, "every other entry exactly as it was, the empty one included")
	assert.Equal(t, uint32(registry.EXPAND_SZ), kind)
}

// What was there is written down before it is changed, so it can be put back by
// hand if everything else fails.
func TestWhatWasThereIsWrittenDownBeforeItIsChanged(t *testing.T) {
	where := scratch(t)
	backupIn(t)
	original := `%SystemRoot%\system32;C:\Other`
	require.NoError(t, writePath(where, original, registry.EXPAND_SZ))
	list := PersistentPathList()

	_, err := changePath(where, User, func(raw string) (string, bool) { return list.Add(raw, `C:\ours`) })
	require.NoError(t, err)

	file, err := backupFile(User)
	require.NoError(t, err)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	var kept pathBackup
	require.NoError(t, json.Unmarshal(content, &kept))

	assert.Equal(t, original, kept.Value, "unresolved, as it was stored")
	assert.Equal(t, uint32(registry.EXPAND_SZ), kept.Kind, "including how it was stored")
	assert.Equal(t, User, kept.Scope)
}

// A change that is not a change records nothing, so the file goes on holding
// the list as it was before this program first touched it.
func TestAnInstallThatChangesNothingDoesNotOverwriteTheRecord(t *testing.T) {
	where := scratch(t)
	backupIn(t)
	ours := `C:\ours`
	require.NoError(t, writePath(where, `C:\first`, registry.EXPAND_SZ))
	list := PersistentPathList()

	_, err := changePath(where, User, func(raw string) (string, bool) { return list.Add(raw, ours) })
	require.NoError(t, err)

	_, err = changePath(where, User, func(raw string) (string, bool) { return list.Add(raw, ours) })
	require.NoError(t, err)

	file, err := backupFile(User)
	require.NoError(t, err)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	var kept pathBackup
	require.NoError(t, json.Unmarshal(content, &kept))
	assert.Equal(t, `C:\first`, kept.Value,
		"the record is of what was there before this program first touched it, not of what it left last time")
}

// Where the record goes, and where it must not. Every value under the account's
// environment key is a variable of that account, so a record kept beside the
// one being changed would become one.
func TestTheRecordIsAFileAndNotAValueInTheEnvironment(t *testing.T) {
	dir := backupIn(t)

	file, err := backupFile(User)

	require.NoError(t, err)
	assert.Equal(t, dir, filepath.Dir(filepath.Dir(file)), "under this account's own data, beside the rendered hook")
	assert.Equal(t, ".json", filepath.Ext(file))
	assert.Contains(t, file, string(User), "a file found on its own says which environment it came out of")
}

// The two scopes are two different environments, and confusing them would have
// a user install reach for the machine's.
func TestTheTwoScopesAreDifferentEnvironments(t *testing.T) {
	mine, err := environmentFor(User)
	require.NoError(t, err)
	everyones, err := environmentFor(Machine)
	require.NoError(t, err)

	assert.Equal(t, registry.CURRENT_USER, mine.root)
	assert.Equal(t, registry.LOCAL_MACHINE, everyones.root)
	assert.NotEqual(t, mine.path, everyones.path)

	_, err = environmentFor("everyone")
	require.Error(t, err)
	assert.Contains(t, err.Error(), string(User))
}

// Telling the system is part of making the change, and a program that is wedged
// must not wedge an install. There is nothing to assert about who listened —
// what this checks is that it is bounded and returns.
func TestAnnouncingTheChangeReturns(t *testing.T) {
	announceEnvironmentChange()
}
