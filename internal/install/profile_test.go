package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two editions are asked separately because they answer differently, so
// the selection is exercised against both of the real answers rather than
// against one specimen standing in for a platform.
func editions(t *testing.T) map[string]Host {
	t.Helper()
	seven, err := parseHost([]byte(powerShell7Answer))
	require.NoError(t, err)
	five, err := parseHost([]byte(windowsPowerShellAnswer))
	require.NoError(t, err)
	return map[string]Host{"PowerShell 7": seven, "Windows PowerShell": five}
}

// Each combination selects a different file. A selection that collapsed two of
// them would wire the wrong sessions while reporting the right ones.
func TestEachScopeAndHostsPairSelectsItsOwnFile(t *testing.T) {
	for edition, host := range editions(t) {
		t.Run(edition, func(t *testing.T) {
			chosen := map[string]string{}
			for _, scope := range []Scope{User, Machine} {
				for _, hosts := range []Hosts{AllHosts, CurrentHost} {
					profile, err := ProfileFor(host, scope, hosts)
					require.NoError(t, err)
					assert.NotContains(t, chosen, profile,
						"%s/%s selected the same file as %s", scope, hosts, chosen[profile])
					chosen[profile] = string(scope) + "/" + string(hosts)
				}
			}
			assert.Len(t, chosen, 4)
		})
	}
}

// Which file each pair means, said once, against the answers a real machine
// gave. These are the paths a user would go and look at.
func TestWhichFileEachPairMeans(t *testing.T) {
	host, err := parseHost([]byte(powerShell7Answer))
	require.NoError(t, err)

	cases := []struct {
		scope Scope
		hosts Hosts
		want  string
	}{
		{User, AllHosts, `C:\Users\example\Documents\PowerShell\profile.ps1`},
		{User, CurrentHost, `C:\Users\example\Documents\PowerShell\Microsoft.PowerShell_profile.ps1`},
		{Machine, AllHosts, `C:\Program Files\PowerShell\7\profile.ps1`},
		{Machine, CurrentHost, `C:\Program Files\PowerShell\7\Microsoft.PowerShell_profile.ps1`},
	}

	for _, c := range cases {
		got, err := ProfileFor(host, c.scope, c.hosts)
		require.NoError(t, err)
		assert.Equal(t, c.want, got, "%s/%s", c.scope, c.hosts)
	}
}

// An account is wired without privilege and a machine is not, so the two must
// never be confused: a user install that reached under Program Files would
// fail for an account that cannot write there, and succeed for one that can by
// wiring every account on the system.
func TestAUserInstallStaysUnderTheAccount(t *testing.T) {
	for edition, host := range editions(t) {
		t.Run(edition, func(t *testing.T) {
			for _, hosts := range []Hosts{AllHosts, CurrentHost} {
				mine, err := ProfileFor(host, User, hosts)
				require.NoError(t, err)
				everyones, err := ProfileFor(host, Machine, hosts)
				require.NoError(t, err)

				assert.Contains(t, mine, `\Users\example\`)
				assert.NotContains(t, everyones, `\Users\example\`)
			}
		})
	}
}

// A host that named no profile for what was asked has not answered it. The
// empty path is a file name, and writing to it would create one in whatever
// directory the install was run from and call that the wiring.
func TestAHostThatNamedNoSuchProfileIsAFailure(t *testing.T) {
	host := Host{Profiles: Profiles{CurrentUserAllHosts: `C:\p.ps1`}}

	_, err := ProfileFor(host, Machine, AllHosts)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to wire")
	assert.Contains(t, err.Error(), string(Machine), "and says which combination went unanswered")
}

// A flag value nobody serves must be refused, not quietly answered with some
// other file: an install that wired a profile the user did not ask for is
// worse than one that did nothing.
func TestACombinationThatIsNotOfferedIsRefusedAndNamesWhatThereIs(t *testing.T) {
	host, err := parseHost([]byte(powerShell7Answer))
	require.NoError(t, err)

	for _, c := range []struct{ scope, hosts string }{
		{"everyone", string(AllHosts)},
		{string(User), "some"},
		{"", ""},
		{"USER", "ALL"},
	} {
		_, err := ProfileFor(host, Scope(c.scope), Hosts(c.hosts))

		require.Error(t, err, "scope %q, hosts %q", c.scope, c.hosts)
		assert.Contains(t, err.Error(), string(User), "the message offers what there is")
		assert.Contains(t, err.Error(), string(CurrentHost))
	}
}

// What an uninstall sweeps. A person may have installed with one set of flags
// and be uninstalling with another, so removing only what today's flags select
// would leave a hook running that nothing mentions any more.
func TestAnUninstallSweepsEveryProfileTheHostHas(t *testing.T) {
	for edition, host := range editions(t) {
		t.Run(edition, func(t *testing.T) {
			swept := EveryProfile(host)

			for _, scope := range []Scope{User, Machine} {
				for _, hosts := range []Hosts{AllHosts, CurrentHost} {
					profile, err := ProfileFor(host, scope, hosts)
					require.NoError(t, err)
					assert.Contains(t, swept, profile,
						"an install can put a hook in %s/%s, so an uninstall has to look there", scope, hosts)
				}
			}
		})
	}
}

// Five names are not five files: what `$PROFILE` alone means is one of the
// other four under most hosts. Sweeping a file twice would have an uninstall
// report removing a wiring it had already removed.
func TestTheSweepNamesEachFileOnce(t *testing.T) {
	for edition, host := range editions(t) {
		t.Run(edition, func(t *testing.T) {
			swept := EveryProfile(host)

			seen := map[string]bool{}
			for _, profile := range swept {
				assert.False(t, seen[profile], "%s was listed twice", profile)
				assert.NotEmpty(t, profile, "a path nobody named is not a file to sweep")
				seen[profile] = true
			}
			assert.Len(t, swept, 4, "the default is the current-host one here, so five names are four files")
		})
	}
}

func TestAHostThatNamedNothingHasNothingToSweep(t *testing.T) {
	assert.Empty(t, EveryProfile(Host{}))
}
