package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestABourneDropInDirectoryThatIsThereIsUsed(t *testing.T) {
	startup := filepath.Join(t.TempDir(), ".bash_profile")
	require.NoError(t, os.Mkdir(BourneDropInDir(startup), 0o755))

	where, err := PlaceBourne(startup, "50-sshakku.sh")

	require.NoError(t, err)
	assert.True(t, where.DropIn)
	assert.Equal(t, filepath.Join(startup+".d", "50-sshakku.sh"), where.Path)
	assert.Empty(t, where.Why, "using the directory needs no explanation")
}

func TestWithNoBourneDropInDirectoryTheBlockGoesInTheFile(t *testing.T) {
	startup := filepath.Join(t.TempDir(), ".bash_profile")

	where, err := PlaceBourne(startup, "50-sshakku.sh")

	require.NoError(t, err)
	assert.False(t, where.DropIn)
	assert.Equal(t, startup, where.Path)
	assert.NoDirExists(t, BourneDropInDir(startup),
		"a directory nothing reads must not be created in order to have one")
}

// The startup file need not exist: an install writes one where there was none.
func TestABourneFileThatIsNotThereYetIsStillWhereTheHookGoes(t *testing.T) {
	startup := filepath.Join(t.TempDir(), ".bash_profile")

	where, err := PlaceBourne(startup, "50-sshakku.sh")

	require.NoError(t, err)
	assert.Equal(t, startup, where.Path)
	assert.NoFileExists(t, startup, "deciding where to write is not writing")
}

// PowerShell loads no drop-in directory of its own. A directory that exists
// says only that somebody made a directory, and a file dropped into one that no
// profile loops over is a hook that never runs while the install reports
// success.
func TestAPowerShellDropInDirectoryIsNotUsedUnlessSomethingLoadsIt(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "Microsoft.PowerShell_profile.ps1")
	require.NoError(t, os.Mkdir(PowerShellDropInDir(profile), 0o755))
	require.NoError(t, os.WriteFile(profile, []byte("Set-Alias ll Get-ChildItem\n"), 0o644))

	where, err := PlacePowerShell(profile, "50-sshakku.ps1")

	require.NoError(t, err)
	assert.False(t, where.DropIn, "the directory is there, and nothing reads it")
	assert.Equal(t, profile, where.Path)
	assert.Contains(t, where.Why, "Profile.d", "and the install has to be able to say why")
	assert.Contains(t, where.Why, "PowerShell does not load such a directory by itself")
}

func TestAPowerShellDropInDirectoryIsUsedWhenTheProfileLoadsIt(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "Microsoft.PowerShell_profile.ps1")
	require.NoError(t, os.Mkdir(PowerShellDropInDir(profile), 0o755))
	require.NoError(t, os.WriteFile(profile, []byte(
		"Get-ChildItem \"$PSScriptRoot\\Profile.d\\*.ps1\" | ForEach-Object { . $_.FullName }\n"), 0o644))

	where, err := PlacePowerShell(profile, "50-sshakku.ps1")

	require.NoError(t, err)
	assert.True(t, where.DropIn)
	assert.Equal(t, filepath.Join(dir, "Profile.d", "50-sshakku.ps1"), where.Path)
	assert.Empty(t, where.Why)
}

// The profile is asked about the user's own content. A block of ours that
// mentioned the directory would otherwise be read as the code that loads it,
// and this would talk itself into a drop-in on the strength of what it wrote.
func TestOurOwnBlockIsNotEvidenceThatTheDirectoryIsLoaded(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "Microsoft.PowerShell_profile.ps1")
	require.NoError(t, os.Mkdir(PowerShellDropInDir(profile), 0o755))
	ours := string(UpsertBlock(nil, `# nothing here loads Profile.d, this line only names it`))
	require.NoError(t, os.WriteFile(profile, []byte(ours), 0o644))

	where, err := PlacePowerShell(profile, "50-sshakku.ps1")

	require.NoError(t, err)
	assert.False(t, where.DropIn)
	assert.NotEmpty(t, where.Why)
}

func TestWithNoPowerShellDropInDirectoryTheBlockGoesInTheProfile(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "Microsoft.PowerShell_profile.ps1")

	where, err := PlacePowerShell(profile, "50-sshakku.ps1")

	require.NoError(t, err)
	assert.False(t, where.DropIn)
	assert.Equal(t, profile, where.Path)
	assert.Empty(t, where.Why, "there was no directory, so there is nothing to explain away")
}

// Something there that is not a directory is neither a drop-in directory nor
// nothing. Reporting it as absent would quietly put the hook in the profile and
// leave the real problem unmentioned.
func TestAFileWhereTheDropInDirectoryShouldBeIsReported(t *testing.T) {
	t.Run("bourne", func(t *testing.T) {
		startup := filepath.Join(t.TempDir(), ".bash_profile")
		require.NoError(t, os.WriteFile(BourneDropInDir(startup), []byte("not a directory"), 0o644))

		_, err := PlaceBourne(startup, "50-sshakku.sh")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a directory")
	})

	t.Run("powershell", func(t *testing.T) {
		profile := filepath.Join(t.TempDir(), "Microsoft.PowerShell_profile.ps1")
		require.NoError(t, os.WriteFile(PowerShellDropInDir(profile), []byte("not a directory"), 0o644))

		_, err := PlacePowerShell(profile, "50-sshakku.ps1")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a directory")
	})
}

func TestNamingNoFileAtAllIsRefused(t *testing.T) {
	_, err := PlaceBourne("", "50-sshakku.sh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nowhere to put the hook")

	_, err = PlacePowerShell("", "50-sshakku.ps1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nowhere to put the hook")
}

// The two families differ on purpose, and this is the assertion that says so:
// the same situation — a drop-in directory beside a startup file nothing in
// which mentions it — is a drop-in for one and a block for the other.
func TestTheTwoFamiliesJudgeTheSameDirectoryDifferently(t *testing.T) {
	dir := t.TempDir()

	bourne := filepath.Join(dir, ".bash_profile")
	require.NoError(t, os.WriteFile(bourne, []byte("export EDITOR=vi\n"), 0o644))
	require.NoError(t, os.Mkdir(BourneDropInDir(bourne), 0o755))

	powershell := filepath.Join(dir, "Microsoft.PowerShell_profile.ps1")
	require.NoError(t, os.WriteFile(powershell, []byte("Set-Alias ll Get-ChildItem\n"), 0o644))
	require.NoError(t, os.Mkdir(PowerShellDropInDir(powershell), 0o755))

	viaShell, err := PlaceBourne(bourne, "50-sshakku.sh")
	require.NoError(t, err)
	viaPowerShell, err := PlacePowerShell(powershell, "50-sshakku.ps1")
	require.NoError(t, err)

	assert.True(t, viaShell.DropIn, "a shell's drop-in directory is read by the shell's own configuration")
	assert.False(t, viaPowerShell.DropIn, "PowerShell's is read by nothing unless the profile says so")
}

// Something that is not a directory where a drop-in directory would be is
// neither a directory nor nothing. Reported as absent, the hook would go into the
// startup file and the real problem would go unmentioned — and the file somebody
// put there would still be there, doing whatever it does.
func TestSomethingThatIsNotADirectoryWhereOneWouldBeIsReported(t *testing.T) {
	dir := t.TempDir()

	bourne := filepath.Join(dir, "profile")
	require.NoError(t, os.WriteFile(BourneDropInDir(bourne), []byte("not a directory"), 0o644))
	_, err := PlaceBourne(bourne, "50-sshakku.sh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")

	powershell := filepath.Join(dir, "Microsoft.PowerShell_profile.ps1")
	require.NoError(t, os.WriteFile(PowerShellDropInDir(powershell), []byte("not a directory"), 0o644))
	_, err = PlacePowerShell(powershell, "50-sshakku.ps1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

// A profile that is not there yet reads nothing, which is not an error: an install
// writes one where there was none. A profile that cannot be read is an error,
// because whether it loads its drop-in directory is then unknown — and guessing
// either way puts the hook somewhere on the strength of nothing.
func TestAProfileThatCannotBeReadIsNotGuessedAbout(t *testing.T) {
	dir := t.TempDir()
	dropIns := filepath.Join(dir, "Profile.d")
	require.NoError(t, os.Mkdir(dropIns, 0o755))

	absent := filepath.Join(dir, "Microsoft.PowerShell_profile.ps1")
	place, err := PlacePowerShell(absent, "50-sshakku.ps1")
	require.NoError(t, err)
	assert.Equal(t, absent, place.Path, "nothing loads a drop-in directory in a profile that does not exist")
	assert.NotEmpty(t, place.Why, "and the directory that was there and unused is why this file was chosen")

	// A directory in the profile's place: not a profile, and not readable as one.
	unreadable := filepath.Join(dir, "profile.ps1")
	require.NoError(t, os.Mkdir(unreadable, 0o755))
	_, err = PlacePowerShell(unreadable, "50-sshakku.ps1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), unreadable)
}

// Something in the way of a path that is not a file or a directory but a name
// that cannot be resolved at all — a directory sought under a file — is neither
// there nor absent, and is reported rather than read as absent.
func TestAPathThatCannotBeReachedIsNotReadAsAbsent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("mine"), 0o644))

	_, err := isDir(filepath.Join(file, "under-a-file"))

	require.Error(t, err, "a name this system cannot resolve is not a directory that is not there")
}
