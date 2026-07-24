package main

import (
	"strings"
	"testing"
)

func TestParseCoverageProfileTotalAndPerPackage(t *testing.T) {
	in := fixture(
		`mode: set`,
		`sshakku/internal/keys/load.go:10.1,12.2 3 1`,
		`sshakku/internal/keys/load.go:14.1,16.2 2 0`,
		`sshakku/internal/config/config.go:5.1,7.2 5 1`,
	)

	total, perPackage, err := parseCoverageProfile(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseCoverageProfile: %v", err)
	}

	// keys: 3 of 5 statements covered = 60%; config: 5 of 5 = 100%.
	// total: 8 of 10 statements covered = 80%.
	if total != 80.0 {
		t.Fatalf("total = %v, want 80.0", total)
	}
	if len(perPackage) != 2 {
		t.Fatalf("len(perPackage) = %d, want 2", len(perPackage))
	}
	if perPackage[0].Package != "sshakku/internal/config" || perPackage[0].Percent != 100.0 {
		t.Fatalf("perPackage[0] = %+v, want {sshakku/internal/config 100}", perPackage[0])
	}
	if perPackage[1].Package != "sshakku/internal/keys" || perPackage[1].Percent != 60.0 {
		t.Fatalf("perPackage[1] = %+v, want {sshakku/internal/keys 60}", perPackage[1])
	}
}

func TestParseCoverageProfileRejectsMalformedInput(t *testing.T) {
	in := fixture(
		`mode: set`,
		`not a valid coverage line`,
	)
	if _, _, err := parseCoverageProfile(strings.NewReader(in)); err == nil {
		t.Fatal("parseCoverageProfile accepted malformed input, want an error")
	}
}

func TestParseCoverageProfileEmptyIsZero(t *testing.T) {
	in := fixture(`mode: set`)
	total, perPackage, err := parseCoverageProfile(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseCoverageProfile: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %v, want 0", total)
	}
	if len(perPackage) != 0 {
		t.Fatalf("len(perPackage) = %d, want 0", len(perPackage))
	}
}
