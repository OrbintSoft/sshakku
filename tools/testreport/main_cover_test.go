package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// badCloser is a ReadCloser whose Close returns closeErr, so the openFile seam
// can hand runRender/runBadge/runSummarize a source that reads fine but fails
// to close — the close-error branch a real *os.File almost never reaches.
type badCloser struct {
	io.Reader
	closeErr error
}

func (b badCloser) Close() error { return b.closeErr }

// errWriter fails every Write, so the report-encoding error path is exercised.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write boom") }

// withOpenFile swaps the openFile seam for the duration of the test.
func withOpenFile(t *testing.T, f func(string) (io.ReadCloser, error)) {
	t.Helper()
	orig := openFile
	openFile = f
	t.Cleanup(func() { openFile = orig })
}

// writeJSON marshals v to a fresh temp file and returns its path.
func writeJSON(t *testing.T, v any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.json")
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// writeFile writes content to a fresh temp file and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func coveredReport() Report {
	return Report{
		OS:              "linux",
		CoveragePercent: 87.5,
		PackageCoverage: []PackageCoverage{{Package: "pkg", Percent: 87.5}},
	}
}

func TestOSURLFlag(t *testing.T) {
	f := osURLFlag{}
	if f.String() != "" {
		t.Fatalf("String() = %q, want empty", f.String())
	}
	if err := f.Set("linux=https://example/x"); err != nil {
		t.Fatalf("Set valid: %v", err)
	}
	if f["linux"] != "https://example/x" {
		t.Fatalf("Set stored %q, want the URL", f["linux"])
	}
	for _, bad := range []string{"noequals", "=url", "os="} {
		if err := f.Set(bad); err == nil {
			t.Fatalf("Set(%q) = nil, want an error", bad)
		}
	}
}

func TestRunDispatch(t *testing.T) {
	report := writeJSON(t, coveredReport())

	t.Run("render subcommand writes the comment body", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if code := run([]string{"render", report}, nil, &out, &errBuf); code != 0 {
			t.Fatalf("code = %d, stderr = %q", code, errBuf.String())
		}
		if !strings.Contains(out.String(), commentMarker) {
			t.Fatalf("render output missing the comment marker:\n%s", out.String())
		}
	})

	t.Run("render failure prints to stderr and exits 1", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if code := run([]string{"render"}, nil, &out, &errBuf); code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(errBuf.String(), "testreport: render:") {
			t.Fatalf("stderr = %q, want a render error", errBuf.String())
		}
	})

	t.Run("badge subcommand writes the badge JSON", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if code := run([]string{"badge", report}, nil, &out, &errBuf); code != 0 {
			t.Fatalf("code = %d, stderr = %q", code, errBuf.String())
		}
		if !strings.Contains(out.String(), "schemaVersion") {
			t.Fatalf("badge output missing schemaVersion:\n%s", out.String())
		}
	})

	t.Run("badge failure prints to stderr and exits 1", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if code := run([]string{"badge"}, nil, &out, &errBuf); code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(errBuf.String(), "testreport: badge:") {
			t.Fatalf("stderr = %q, want a badge error", errBuf.String())
		}
	})

	t.Run("no subcommand summarizes stdin", func(t *testing.T) {
		in := fixture(
			`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestFoo"}`,
			`{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"pkg","Test":"TestFoo","Elapsed":1.0}`,
		)
		var out, errBuf bytes.Buffer
		if code := run([]string{"-os", "linux"}, strings.NewReader(in), &out, &errBuf); code != 0 {
			t.Fatalf("code = %d, stderr = %q", code, errBuf.String())
		}
		if !strings.Contains(out.String(), `"os": "linux"`) {
			t.Fatalf("summarize output missing the OS label:\n%s", out.String())
		}
	})

	t.Run("summarize with empty args still reads stdin", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if code := run(nil, strings.NewReader(""), &out, &errBuf); code != 0 {
			t.Fatalf("code = %d, stderr = %q", code, errBuf.String())
		}
	})

	t.Run("summarize failure prints to stderr and exits 1", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if code := run([]string{"-nonexistent-flag"}, strings.NewReader(""), &out, &errBuf); code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(errBuf.String(), "testreport:") {
			t.Fatalf("stderr = %q, want a testreport error", errBuf.String())
		}
	})
}

func TestRunRender(t *testing.T) {
	t.Run("bad flags surface", func(t *testing.T) {
		if err := runRender([]string{"-nonexistent-flag"}, io.Discard); err == nil {
			t.Fatal("expected an error for an unknown flag")
		}
	})

	t.Run("no report paths is a usage error", func(t *testing.T) {
		if err := runRender(nil, io.Discard); err == nil {
			t.Fatal("expected a usage error with no report paths")
		}
	})

	t.Run("a missing file surfaces", func(t *testing.T) {
		if err := runRender([]string{filepath.Join(t.TempDir(), "nope.json")}, io.Discard); err == nil {
			t.Fatal("expected an error opening a missing report")
		}
	})

	t.Run("undecodable JSON surfaces", func(t *testing.T) {
		path := writeFile(t, "bad.json", "not json")
		if err := runRender([]string{path}, io.Discard); err == nil {
			t.Fatal("expected a decode error")
		}
	})

	t.Run("a close error surfaces", func(t *testing.T) {
		withOpenFile(t, func(string) (io.ReadCloser, error) {
			return badCloser{Reader: strings.NewReader(`{"os":"linux"}`), closeErr: errors.New("close boom")}, nil
		})
		err := runRender([]string{"any.json"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "close boom") {
			t.Fatalf("err = %v, want a close error", err)
		}
	})

	t.Run("an output write failure surfaces", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		if err := runRender([]string{path}, errWriter{}); err == nil {
			t.Fatal("expected an error writing the report to a failing writer")
		}
	})

	t.Run("per-OS artifact URLs parse and render", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		var out bytes.Buffer
		err := runRender([]string{"-report-url", "linux=https://example/r", "-coverage-url", "linux=https://example/c", path}, &out)
		if err != nil {
			t.Fatalf("runRender: %v", err)
		}
		if !strings.Contains(out.String(), "https://example/r") || !strings.Contains(out.String(), "https://example/c") {
			t.Fatalf("output missing the artifact URLs:\n%s", out.String())
		}
	})
}

func TestRunBadge(t *testing.T) {
	t.Run("wrong argument count is a usage error", func(t *testing.T) {
		if err := runBadge(nil, io.Discard); err == nil {
			t.Fatal("expected a usage error with no arguments")
		}
	})

	t.Run("a missing file surfaces", func(t *testing.T) {
		if err := runBadge([]string{filepath.Join(t.TempDir(), "nope.json")}, io.Discard); err == nil {
			t.Fatal("expected an error opening a missing report")
		}
	})

	t.Run("undecodable JSON surfaces", func(t *testing.T) {
		path := writeFile(t, "bad.json", "not json")
		if err := runBadge([]string{path}, io.Discard); err == nil {
			t.Fatal("expected a decode error")
		}
	})

	t.Run("a report with no coverage surfaces", func(t *testing.T) {
		path := writeJSON(t, Report{OS: "linux"})
		if err := runBadge([]string{path}, io.Discard); err == nil {
			t.Fatal("expected an error rendering a badge with no coverage data")
		}
	})

	t.Run("a close error surfaces", func(t *testing.T) {
		withOpenFile(t, func(string) (io.ReadCloser, error) {
			return badCloser{Reader: strings.NewReader(`{"os":"linux","package_coverage":[{"package":"p","percent":50}]}`), closeErr: errors.New("close boom")}, nil
		})
		err := runBadge([]string{"any.json"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "close boom") {
			t.Fatalf("err = %v, want a close error", err)
		}
	})

	t.Run("success writes the badge JSON", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		var out bytes.Buffer
		if err := runBadge([]string{path}, &out); err != nil {
			t.Fatalf("runBadge: %v", err)
		}
		if !strings.Contains(out.String(), "schemaVersion") {
			t.Fatalf("output missing schemaVersion:\n%s", out.String())
		}
	})

	t.Run("an output write failure surfaces", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		if err := runBadge([]string{path}, errWriter{}); err == nil {
			t.Fatal("expected an error writing the badge to a failing writer")
		}
	})
}

func TestRunSummarize(t *testing.T) {
	validStream := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestFoo"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"pkg","Test":"TestFoo","Elapsed":1.0}`,
	)

	t.Run("bad flags surface", func(t *testing.T) {
		if err := runSummarize([]string{"-nonexistent-flag"}, strings.NewReader(""), io.Discard); err == nil {
			t.Fatal("expected an error for an unknown flag")
		}
	})

	t.Run("an unparsable event stream surfaces", func(t *testing.T) {
		if err := runSummarize(nil, strings.NewReader("not json\n"), io.Discard); err == nil {
			t.Fatal("expected an error parsing a bad event stream")
		}
	})

	t.Run("a missing coverage profile surfaces", func(t *testing.T) {
		err := runSummarize([]string{"-coverprofile", filepath.Join(t.TempDir(), "nope.cover")}, strings.NewReader(validStream), io.Discard)
		if err == nil {
			t.Fatal("expected an error opening a missing coverage profile")
		}
	})

	t.Run("a malformed coverage profile surfaces", func(t *testing.T) {
		profile := writeFile(t, "bad.cover", "mode: set\nfile.go:1.1,2.2 notanumber 1\n")
		err := runSummarize([]string{"-coverprofile", profile}, strings.NewReader(validStream), io.Discard)
		if err == nil {
			t.Fatal("expected an error parsing a malformed coverage profile")
		}
	})

	t.Run("a coverage-profile close error surfaces", func(t *testing.T) {
		withOpenFile(t, func(string) (io.ReadCloser, error) {
			return badCloser{Reader: strings.NewReader("mode: set\n"), closeErr: errors.New("close boom")}, nil
		})
		err := runSummarize([]string{"-coverprofile", "any.cover"}, strings.NewReader(validStream), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "close boom") {
			t.Fatalf("err = %v, want a close error", err)
		}
	})

	t.Run("success folds in coverage", func(t *testing.T) {
		profile := writeFile(t, "ok.cover", "mode: set\nsshakku/pkg/x.go:1.1,2.2 4 1\n")
		var out bytes.Buffer
		if err := runSummarize([]string{"-os", "linux", "-coverprofile", profile}, strings.NewReader(validStream), &out); err != nil {
			t.Fatalf("runSummarize: %v", err)
		}
		if !strings.Contains(out.String(), `"coverage_percent"`) {
			t.Fatalf("output missing coverage:\n%s", out.String())
		}
	})

	t.Run("success without a coverage profile", func(t *testing.T) {
		var out bytes.Buffer
		if err := runSummarize([]string{"-os", "linux"}, strings.NewReader(validStream), &out); err != nil {
			t.Fatalf("runSummarize: %v", err)
		}
		if !strings.Contains(out.String(), `"os": "linux"`) {
			t.Fatalf("output missing the OS label:\n%s", out.String())
		}
	})

	t.Run("an output write failure surfaces", func(t *testing.T) {
		if err := runSummarize([]string{"-os", "linux"}, strings.NewReader(validStream), errWriter{}); err == nil {
			t.Fatal("expected an error encoding the report to a failing writer")
		}
	})
}
