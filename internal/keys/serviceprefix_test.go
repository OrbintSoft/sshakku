package keys

import "testing"

// TestOwnServicesGoesByThePrefixItIsGiven pins the half of the entry name that
// says who stored it. The case that matters is the second one: under a
// configuration naming one prefix, an entry carrying a different one is not
// sshakku's — not even the default's — because nothing this configuration can
// write would ever have produced it.
func TestOwnServicesGoesByThePrefixItIsGiven(t *testing.T) {
	const mine = "wallet-of-mine"
	names := []string{
		"github.com",
		mine + "-id_ed25519",
		defaultServicePrefix + "-id_rsa",
		"Passport scan",
	}

	t.Run("a configured prefix keeps its own entries and no others", func(t *testing.T) {
		got := ownServices(names, mine)
		want := []string{mine + "-id_ed25519"}
		if !equalStrings(got, want) {
			t.Fatalf("ownServices = %v, want %v", got, want)
		}
	})

	t.Run("no prefix configured falls back to the default", func(t *testing.T) {
		got := ownServices(names, "")
		want := []string{defaultServicePrefix + "-id_rsa"}
		if !equalStrings(got, want) {
			t.Fatalf("ownServices = %v, want %v", got, want)
		}
	})
}

// TestServicePrefixOrDefault covers the single place that decides what an unset
// prefix means, which is why writing an entry and enumerating one cannot
// disagree about it.
func TestServicePrefixOrDefault(t *testing.T) {
	if got := servicePrefixOrDefault(""); got != defaultServicePrefix {
		t.Errorf("servicePrefixOrDefault(\"\") = %q, want %q", got, defaultServicePrefix)
	}
	if got := servicePrefixOrDefault("chosen"); got != "chosen" {
		t.Errorf("servicePrefixOrDefault(%q) = %q, want it kept", "chosen", got)
	}
	if got := servicePrefixOf(Config{ServicePrefix: "chosen"}); got != "chosen" {
		t.Errorf("servicePrefixOf = %q, want the config's own prefix", got)
	}
}

// TestBitwardenListGoesByTheBackendsOwnPrefix verifies F32 where the vault is
// shared with everything else its owner keeps there: what List reports is what
// `forget --all` deletes, so it must report the entries the configured prefix
// names and nothing else — including nothing under the default, which under
// this configuration is another program's name as much as "Bank" is.
func TestBitwardenListGoesByTheBackendsOwnPrefix(t *testing.T) {
	const mine = "wallet-of-mine"
	r := newFakeRunner().on(bitwardenBin, stdout(
		`[{"name":"Bank"},{"name":"`+mine+`-id_ed25519"},{"name":"`+defaultServicePrefix+`-id_rsa"}]`, 0))
	b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true, ServicePrefix: mine}

	got, err := b.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{mine + "-id_ed25519"}
	if !equalStrings(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
}
