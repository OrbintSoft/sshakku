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

// envValue renders a variable's value, or says there is none. The placeholder
// is parenthesised like the report's others ((none)), marking a statement
// about a value rather than a value itself.
func envValue(value string) string {
	if value == "" {
		return "(unset)"
	}
	return value
}

// secretEnvState renders whether a secret is set, and never anything more.
func secretEnvState(set bool) string {
	if set {
		return "(set)"
	}
	return "(unset)"
}
