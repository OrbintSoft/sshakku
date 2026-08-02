package keys

import (
	"testing"
)

// TestKeePassXCCLIUsesTheConfiguredContainer verifies the KeePassXC half of the
// promise that the compartment SSHakku keeps its entries in is the one the
// configuration names (F33): the group it creates, the group each entry is
// filed under, and the group it enumerates are all that one.
//
// It reads the argv that crossed the process boundary rather than the backend's
// intent, because a group name that is only half applied — created here, read
// back there — is exactly the shape of failure that looks like an empty wallet.
func TestKeePassXCCLIUsesTheConfiguredContainer(t *testing.T) {
	const group = "my-own-compartment"
	const service = "SSHakku-Key-id_rsa"

	t.Run("a new entry creates and fills the configured group", func(t *testing.T) {
		// The first call is Store's own lookup; a non-zero exit is the miss that
		// makes it create the group rather than edit an entry.
		runner := &recordingRunner{results: []Result{{Code: 1}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})
		b.Group = group

		if err := b.Store(service, "label", "hunter2"); err != nil {
			t.Fatalf("Store: %v", err)
		}
		if len(runner.calls) != 3 {
			t.Fatalf("ran %d commands, want 3 (lookup, mkdir, add)", len(runner.calls))
		}
		if got := lastArg(runner.calls[1].Args); got != group {
			t.Errorf("created group %q, want %q", got, group)
		}
		if got, want := lastArg(runner.calls[2].Args), group+"/"+service; got != want {
			t.Errorf("filed the entry at %q, want %q", got, want)
		}
	})

	t.Run("a lookup reads from the configured group", func(t *testing.T) {
		runner := &recordingRunner{results: []Result{{Code: 0, Stdout: []byte("hunter2\n")}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})
		b.Group = group

		if _, _, err := b.Lookup(service); err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if got, want := lastArg(runner.calls[0].Args), group+"/"+service; got != want {
			t.Errorf("looked up %q, want %q", got, want)
		}
	})

	t.Run("a sweep enumerates the configured group and reports plain names", func(t *testing.T) {
		runner := &recordingRunner{results: []Result{{Code: 0, Stdout: []byte(group + "/" + service + "\n")}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})
		b.Group = group

		services, err := b.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if got := lastArg(runner.calls[0].Args); got != group {
			t.Errorf("enumerated group %q, want %q", got, group)
		}
		// The group name is where the entry lives, not part of what it is
		// called: a sweep that left it on would hand `forget --all` names no
		// lookup could ever match.
		if len(services) != 1 || services[0] != service {
			t.Errorf("List = %q, want exactly %q", services, []string{service})
		}
	})

	t.Run("an unset name keeps the group SSHakku has always used", func(t *testing.T) {
		runner := &recordingRunner{results: []Result{{Code: 0, Stdout: []byte("hunter2\n")}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})

		if _, _, err := b.Lookup(service); err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if got, want := lastArg(runner.calls[0].Args), keepassxcCLIGroup+"/"+service; got != want {
			t.Errorf("looked up %q, want the default group's %q", got, want)
		}
	})
}

// lastArg is the argument keepassxc-cli takes as the path to act on, which is
// always the final one.
func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}
