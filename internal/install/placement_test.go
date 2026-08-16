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
