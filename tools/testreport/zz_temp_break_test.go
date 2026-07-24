package main

import "testing"

// Temporary: deliberately fails to verify the pr-test-report Failures
// section renders correctly on a real PR. Removed in the very next commit.
func TestZZTemporaryDeliberateFailure(t *testing.T) {
	t.Fatal("deliberate failure for sub-step 5 end-to-end verification")
}
