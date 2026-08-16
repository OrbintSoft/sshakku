package install

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// windowsList is the other system's rule, written out here so that what an
// install does to a list on that system can be checked from any machine. The
// rules are the interesting part and neither is this file's platform's.
var windowsList = PathList{
	Separator: ";",
	SameEntry: func(a, b string) bool {
		return strings.EqualFold(trimTrailingSeparators(a, `\/`), trimTrailingSeparators(b, `\/`))
	},
}

const ours = `C:\Users\example\AppData\Local\Programs\sshakku`

func TestAnEntryIsAddedAtTheEnd(t *testing.T) {
	got, changed := windowsList.Add(`C:\Windows;C:\Windows\System32`, ours)

	assert.True(t, changed)
	assert.Equal(t, `C:\Windows;C:\Windows\System32;`+ours, got,
		"at the end: an entry at the front of a persistent list changes which program every other name resolves to")
}

func TestAddingToAnEmptyListIsJustTheEntry(t *testing.T) {
	for _, empty := range []string{"", "   "} {
		got, changed := windowsList.Add(empty, ours)

		assert.True(t, changed)
		assert.Equal(t, ours, got, "and never a leading separator, which would be an empty entry")
	}
}

// However many times an install is run, the entry is there once.
func TestAddingAnEntryThatIsAlreadyThereChangesNothing(t *testing.T) {
	list := `C:\Windows;` + ours + `;C:\Other`

	for _, spelling := range []string{
		ours,
		strings.ToUpper(ours),
		ours + `\`,
		ours + `/`,
	} {
		got, changed := windowsList.Add(list, spelling)

		assert.False(t, changed, "%q is the same directory", spelling)
		assert.Equal(t, list, got)
	}
}

// The stored value refers to other variables on purpose: it means whatever they
// mean at the moment it is read. Resolving them during an install would freeze
// them into what they happened to mean that day.
func TestEntriesThatReferToVariablesAreLeftExactlyAsTheyAre(t *testing.T) {
	list := `%SystemRoot%\system32;%USERPROFILE%\bin;%JAVA_HOME%\bin`

	got, changed := windowsList.Add(list, ours)

	require.True(t, changed)
	assert.True(t, strings.HasPrefix(got, list), "nothing before the new entry may be rewritten")
	assert.Contains(t, got, "%SystemRoot%")
	assert.Contains(t, got, "%USERPROFILE%")
	assert.Contains(t, got, "%JAVA_HOME%")
}

// A list ending in a separator must not gain a second one: two separators
// together are an empty entry, and on that system an empty entry is the current
// directory — a real change to what every session searches.
func TestAListThatAlreadyEndsInASeparatorDoesNotGainAnother(t *testing.T) {
	got, changed := windowsList.Add(`C:\Windows;`, ours)

	assert.True(t, changed)
	assert.Equal(t, `C:\Windows;`+ours, got)
}

func TestRemovingTakesTheEntryAndItsSeparatorTogether(t *testing.T) {
	got, changed := windowsList.Remove(`C:\Windows;`+ours+`;C:\Other`, ours)

	assert.True(t, changed)
	assert.Equal(t, `C:\Windows;C:\Other`, got, "and leaves no empty entry where ours used to be")
}

func TestRemovingTheOnlyEntryLeavesNothing(t *testing.T) {
	got, changed := windowsList.Remove(ours, ours)

	assert.True(t, changed)
	assert.Empty(t, got)
}

func TestRemovingSomethingThatIsNotThereChangesNothing(t *testing.T) {
	list := `C:\Windows;C:\Other`

	got, changed := windowsList.Remove(list, ours)

	assert.False(t, changed)
	assert.Equal(t, list, got)
}

// The promise an uninstall makes: every other entry exactly as it was. Empty
// entries included — an empty entry means something, and losing one silently
// changes what every session searches.
func TestRemovingLeavesEveryOtherEntryExactlyAsItWas(t *testing.T) {
	list := `%SystemRoot%\system32;;C:\Program Files\Git\cmd;` + ours + `;C:\Windows\;D:\tools`

	got, changed := windowsList.Remove(list, ours)

	require.True(t, changed)
	assert.Equal(t, `%SystemRoot%\system32;;C:\Program Files\Git\cmd;C:\Windows\;D:\tools`, got)
}

func TestRemovingFindsTheEntryHoweverItWasSpelt(t *testing.T) {
	for _, spelling := range []string{strings.ToUpper(ours), ours + `\`, ours + `/`} {
		got, changed := windowsList.Remove(`C:\Windows;`+spelling+`;C:\Other`, ours)

		assert.True(t, changed, "%q is the same directory", spelling)
		assert.Equal(t, `C:\Windows;C:\Other`, got)
	}
}

// An install followed by an uninstall has to give back what was there, or the
// two are not each other's opposite.
func TestAddingThenRemovingGivesBackWhatWasThere(t *testing.T) {
	for _, before := range []string{
		"",
		`C:\Windows`,
		`C:\Windows;`,
		`%SystemRoot%\system32;;C:\Program Files\Git\cmd`,
	} {
		added, changed := windowsList.Add(before, ours)
		require.True(t, changed, "%q", before)

		after, changed := windowsList.Remove(added, ours)

		require.True(t, changed)
		assert.Equal(t, strings.TrimSuffix(before, ";"), strings.TrimSuffix(after, ";"),
			"starting from %q", before)
	}
}

// The empty string is not a directory, and must not be taken for one: asked to
// add it there is nothing to add, and asked to remove it the empty entries in
// the list are somebody else's and stay.
func TestTheEmptyEntryIsNobodysDirectory(t *testing.T) {
	list := `C:\Windows;;C:\Other`

	got, changed := windowsList.Add(list, "")
	assert.False(t, changed)
	assert.Equal(t, list, got)

	got, changed = windowsList.Remove(list, "")
	assert.False(t, changed)
	assert.Equal(t, list, got, "an empty entry means the current directory, and is not ours to remove")
}

// Whatever this system's own rule is, these hold of it.
func TestThisSystemsOwnListRule(t *testing.T) {
	list := PersistentPathList()
	require.NotEmpty(t, list.Separator)
	require.NotNil(t, list.SameEntry)

	added, changed := list.Add("", "/somewhere/bin")
	require.True(t, changed)
	assert.Equal(t, "/somewhere/bin", added)

	twice, changed := list.Add(added, "/somewhere/bin")
	assert.False(t, changed, "an install run twice adds its entry once")
	assert.Equal(t, added, twice)

	back, changed := list.Remove(added, "/somewhere/bin")
	assert.True(t, changed)
	assert.Empty(t, back)
}

// A root is separators all the way down, and trimming it to nothing would make
// it match everything.
func TestARootIsNotTrimmedAway(t *testing.T) {
	assert.Equal(t, `\`, trimTrailingSeparators(`\`, `\/`))
	assert.Equal(t, "/", trimTrailingSeparators("/", `\/`))
	assert.Equal(t, `C:`, trimTrailingSeparators(`C:\`, `\/`))
}
