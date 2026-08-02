package diagnose

// EnvVar is one environment variable the report shows, with the value the
// shell gave it.
type EnvVar struct {
	Name  string
	Value string
}

// SecretEnvVar is one environment variable whose value is a secret: a session
// token, a master password. The report says whether it is set and there is
// deliberately nowhere here to put the value, so no way of rendering a report
// can disclose one. A report is what a user pastes into a bug report.
type SecretEnvVar struct {
	Name string
	Set  bool
}

// envNotReadable is what a variable shows as when the environment being
// reported on is not this process's own — a report about another user's
// session. It is not the same answer as "unset": that is a fact about a shell
// somebody looked at, and this is the absence of one.
// Parenthesised like the report's other placeholders ((none), (unset)), which
// mark a statement about a value rather than a value itself.
const envNotReadable = "(not readable from here)"

// envValue renders a variable's value, or why there is none to show.
func envValue(value string, unreadable bool) string {
	switch {
	case unreadable:
		return envNotReadable
	case value == "":
		return "(unset)"
	}
	return value
}

// secretEnvState renders whether a secret is set, and never anything more.
func secretEnvState(set, unreadable bool) string {
	switch {
	case unreadable:
		return envNotReadable
	case set:
		return "(set)"
	}
	return "(unset)"
}
