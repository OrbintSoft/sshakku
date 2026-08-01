package keys

// Shared by the backend tests, which all compare a List result against what
// was stored. It lives in a file of its own so it belongs to no one platform:
// the backends that use it do not all exist on the same one.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
