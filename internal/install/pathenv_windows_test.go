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
		backupIn(t)
		require.NoError(t, writePath(where, `C:\one`, kind))

		list := PersistentPathList()
		_, err := changePath(where, User, recordPrevious, func(raw string) (string, bool) { return list.Add(raw, `C:\two`) })
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

	changed, err := changePath(where, User, recordPrevious, func(raw string) (string, bool) { return list.Add(raw, ours) })
	require.NoError(t, err)
	assert.True(t, changed)

	after, _, err := readPath(where)
	require.NoError(t, err)
	assert.Equal(t, original+";"+ours, after)

	changed, err = changePath(where, User, recordPrevious, func(raw string) (string, bool) { return list.Add(raw, ours) })
	require.NoError(t, err)
	assert.False(t, changed, "however many times an install is run, the entry is there once")

	changed, err = changePath(where, User, recordPrevious, func(raw string) (string, bool) { return list.Remove(raw, ours) })
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

	_, err := changePath(where, User, recordPrevious, func(raw string) (string, bool) { return list.Add(raw, `C:\ours`) })
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

// The record answers one question: what was the list before this program first
// put itself into it. Taking the entry back out changes the list too, and
// recording there would answer that question with a list that has this
// program's own entry in it — the one value somebody restoring by hand must
// never be handed.
//
// Driven through the two operations themselves rather than through changePath,
// because which of them writes the record is the decision under test: passing
// the flag in from here would assert the answer instead of asking for it.
func TestTakingTheEntryBackOutDoesNotRewriteTheRecord(t *testing.T) {
	where := scratch(t)
	backupIn(t)
	original := `%SystemRoot%\system32;C:\Program Files\Git\cmd`
	require.NoError(t, writePath(where, original, registry.EXPAND_SZ))
	ours := `C:\Users\someone\AppData\Local\Programs\sshakku`

	added, err := addToPathIn(where, User, ours)
	require.NoError(t, err)
	require.True(t, added)
	removed, err := removeFromPathIn(where, User, ours)
	require.NoError(t, err)
	require.True(t, removed, "there must have been something to take out")

	file, err := backupFile(User)
	require.NoError(t, err)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	var kept pathBackup
	require.NoError(t, json.Unmarshal(content, &kept))

	assert.Equal(t, original, kept.Value, "the record still says what was there before the install")
	assert.NotContains(t, kept.Value, ours, "and never names this program's own entry")
}

// A change that is not a change records nothing, so the file goes on holding
// the list as it was before this program first touched it.
func TestAnInstallThatChangesNothingDoesNotOverwriteTheRecord(t *testing.T) {
	where := scratch(t)
	backupIn(t)
	ours := `C:\ours`
	require.NoError(t, writePath(where, `C:\first`, registry.EXPAND_SZ))
	list := PersistentPathList()

	_, err := changePath(where, User, recordPrevious, func(raw string) (string, bool) { return list.Add(raw, ours) })
	require.NoError(t, err)

	_, err = changePath(where, User, recordPrevious, func(raw string) (string, bool) { return list.Add(raw, ours) })
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

// noSuchKey names a key that is not there, and makes sure of it.
//
// An account that has never had a search list of its own is not this: a value
// that is missing from a key that exists is an empty list. This is the key
// itself being unopenable, which is what a scope somebody removed looks like,
// and what every refusal by permission looks like from in here.
func noSuchKey(t *testing.T) environmentLocation {
	t.Helper()

	where := environmentLocation{
		root: registry.CURRENT_USER,
		path: fmt.Sprintf(`Software\SSHakku\test-%d-no-such-key`, os.Getpid()),
	}
	_, err := registry.OpenKey(where.root, where.path, registry.QUERY_VALUE)
	require.Error(t, err, "this key must not exist, or the tests below prove nothing")
	return where
}

// F44: a step that could not be taken says which step it was, and here that
// means naming the key — because what a person does next is go and look at it.
func TestASearchListThatCannotBeReadSaysWhereItWasLookedFor(t *testing.T) {
	t.Run("nothing there to open", func(t *testing.T) {
		where := noSuchKey(t)

		_, _, err := readPath(where)

		require.Error(t, err)
		assert.Contains(t, err.Error(), where.path, "which key could not be opened is the whole of the answer")
		assert.Contains(t, err.Error(), "read", "and what was being done to it when it refused")
	})

	// The one that matters most of all. A value stored as something other than
	// text is not an absent one, and answering as though it were would hand back
	// an empty list — which the caller would then add this program's directory
	// to and write back, replacing the account's entire search list with one
	// entry. It is refused instead, and the account's own value is left alone.
	t.Run("a search list stored as something that is not text", func(t *testing.T) {
		where := scratch(t)
		key, err := registry.OpenKey(where.root, where.path, registry.SET_VALUE)
		require.NoError(t, err)
		require.NoError(t, key.SetDWordValue(pathValue, 1))
		require.NoError(t, key.Close())

		raw, _, err := readPath(where)

		require.Error(t, err, "a list that cannot be read is not an empty list")
		assert.Empty(t, raw)
		assert.Contains(t, err.Error(), where.path)
	})
}

// The same for writing, and with it the promise that nothing is half-written:
// a list this cannot store is left as it was found.
func TestASearchListThatCannotBeWrittenSaysWhereItWouldHaveGone(t *testing.T) {
	t.Run("nothing there to open", func(t *testing.T) {
		where := noSuchKey(t)

		err := writePath(where, `C:\somewhere`, registry.EXPAND_SZ)

		require.Error(t, err)
		assert.Contains(t, err.Error(), where.path)
		assert.Contains(t, err.Error(), "write", "and what was being done to it when it refused")
	})

	// A list with a NUL in it is not a list this system can store as text, and
	// the conversion is where that is met. Nothing that reads a real search list
	// produces one — which is the point: the value arrives from a caller, and a
	// caller that hands over something unstorable gets told so rather than
	// having it stored in some other shape.
	t.Run("a list this system cannot store as text", func(t *testing.T) {
		where := scratch(t)
		original := `%SystemRoot%\system32;C:\Program Files\Git\cmd`
		require.NoError(t, writePath(where, original, registry.EXPAND_SZ))

		err := writePath(where, "C:\\one\x00C:\\two", registry.EXPAND_SZ)

		require.Error(t, err)
		raw, kind, readErr := readPath(where)
		require.NoError(t, readErr)
		assert.Equal(t, original, raw, "what was there is still there, whole")
		assert.Equal(t, uint32(registry.EXPAND_SZ), kind)
	})
}

// F47: the record of what the search list was is what an administrator puts
// back by hand, so a change that cannot be written down is not made at all. The
// list reads afterwards exactly as it read before — the alternative is an
// account's search list altered with no record anywhere of what it had been.
func TestAChangeThatCannotBeWrittenDownIsNotMade(t *testing.T) {
	original := `%SystemRoot%\system32;C:\Program Files\Git\cmd`

	cases := map[string]func(t *testing.T){
		"the environment names nowhere to keep it": func(t *testing.T) {
			t.Helper()
			t.Setenv("LOCALAPPDATA", "")
		},
		"a file where the directory would go": func(t *testing.T) {
			t.Helper()
			dir := backupIn(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "sshakku"), []byte("somebody's own"), 0o600))
		},
		"a directory where the file would go": func(t *testing.T) {
			t.Helper()
			dir := backupIn(t)
			taken, err := backupFile(User)
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(taken, 0o750), "under "+dir)
		},
	}
	for name, inTheWay := range cases {
		t.Run(name, func(t *testing.T) {
			where := scratch(t)
			require.NoError(t, writePath(where, original, registry.EXPAND_SZ))
			inTheWay(t)
			list := PersistentPathList()

			changed, err := changePath(where, User, recordPrevious, func(raw string) (string, bool) {
				return list.Add(raw, `C:\ours`)
			})

			require.Error(t, err)
			assert.False(t, changed, "a change that was refused is not one to report as made")
			raw, _, readErr := readPath(where)
			require.NoError(t, readErr)
			assert.Equal(t, original, raw, "and the account's search list is what it was")
		})
	}
}

// A list that could not be read is not one to write over. What would be written
// is built from what was read, so carrying on from a failed read would put this
// program's directory where an account's whole search list used to be.
func TestAListThatCouldNotBeReadIsNotWritten(t *testing.T) {
	where := noSuchKey(t)
	list := PersistentPathList()

	changed, err := changePath(where, User, recordPrevious, func(raw string) (string, bool) {
		return list.Add(raw, `C:\ours`)
	})

	require.Error(t, err)
	assert.False(t, changed)
	_, openErr := registry.OpenKey(where.root, where.path, registry.QUERY_VALUE)
	assert.Error(t, openErr, "and nothing was created on the way past")
}

// The key going away between the read and the write is the one failure that
// cannot be arranged by pointing somewhere else, since the same location has to
// answer once and then refuse. It is reported as a change not made, naming the
// key, rather than reported as made.
func TestAKeyThatGoesAwayMidChangeIsReportedAsAChangeNotMade(t *testing.T) {
	where := scratch(t)
	require.NoError(t, writePath(where, `C:\one`, registry.EXPAND_SZ))
	list := PersistentPathList()

	changed, err := changePath(where, User, leaveRecord, func(raw string) (string, bool) {
		require.NoError(t, registry.DeleteKey(where.root, where.path), "taken away after the read and before the write")
		return list.Add(raw, `C:\ours`)
	})

	require.Error(t, err)
	assert.False(t, changed)
	assert.Contains(t, err.Error(), where.path)
}

// A scope with no environment behind it is refused before anything is opened,
// and the refusal names the two there are. Both directions are asked, because
// an uninstall reaches this by the other door.
func TestAScopeNobodyServesIsRefusedWhicheverDirectionItCameFrom(t *testing.T) {
	for name, change := range map[string]func(Scope, string) (bool, error){
		"adding":   AddToPath,
		"removing": RemoveFromPath,
	} {
		t.Run(name, func(t *testing.T) {
			// Safe to call the real entry points with this: a scope that names no
			// environment is refused before one is opened, so the account's own is
			// never reached. Any scope that did name one would be the live
			// environment of the person running the tests.
			changed, err := change("everyone", `C:\somewhere`)

			require.Error(t, err)
			assert.False(t, changed)
			assert.Contains(t, err.Error(), string(User), "and says which scopes there are")
			assert.Contains(t, err.Error(), string(Machine))
		})
	}
}
