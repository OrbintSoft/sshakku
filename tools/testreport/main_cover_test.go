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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "marshal the fixture report")
	require.NoError(t, os.WriteFile(path, data, 0o600), "write the fixture report")
	return path
}

// writeFile writes content to a fresh temp file and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600), "write the fixture file")
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
	assert.Empty(t, f.String(), "a flag nothing was given to has nothing to show")

	require.NoError(t, f.Set("linux=https://example/x"), "Set a well-formed os=url pair")
	assert.Equal(t, "https://example/x", f["linux"], "the URL must be stored under its OS")

	for _, bad := range []string{"noequals", "=url", "os="} {
		assert.Errorf(t, f.Set(bad), "%q is not an os=url pair and must be refused", bad)
	}
}

func TestRunDispatch(t *testing.T) {
	report := writeJSON(t, coveredReport())

	t.Run("render subcommand writes the comment body", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := run([]string{"render", report}, nil, &out, &errBuf)
		require.Equalf(t, 0, code, "render must succeed, stderr: %q", errBuf.String())
		assert.Contains(t, out.String(), commentMarker, "the comment body must carry the marker")
	})

	t.Run("render failure prints to stderr and exits 1", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := run([]string{"render"}, nil, &out, &errBuf)
		assert.Equal(t, 1, code, "a render that could not be done must exit non-zero")
		assert.Contains(t, errBuf.String(), "testreport: render:", "the error must say which subcommand failed")
	})

	t.Run("badge subcommand writes the badge JSON", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := run([]string{"badge", report}, nil, &out, &errBuf)
		require.Equalf(t, 0, code, "badge must succeed, stderr: %q", errBuf.String())
		assert.Contains(t, out.String(), "schemaVersion", "the badge must be the shape shields.io reads")
	})

	t.Run("badge failure prints to stderr and exits 1", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := run([]string{"badge"}, nil, &out, &errBuf)
		assert.Equal(t, 1, code, "a badge that could not be made must exit non-zero")
		assert.Contains(t, errBuf.String(), "testreport: badge:", "the error must say which subcommand failed")
	})

	t.Run("no subcommand summarizes stdin", func(t *testing.T) {
		in := fixture(
			`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestFoo"}`,
			`{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"pkg","Test":"TestFoo","Elapsed":1.0}`,
		)
		var out, errBuf bytes.Buffer
		code := run([]string{"-os", "linux"}, strings.NewReader(in), &out, &errBuf)
		require.Equalf(t, 0, code, "summarize must succeed, stderr: %q", errBuf.String())
		assert.Contains(t, out.String(), `"os": "linux"`, "the summary must say which OS it is from")
	})

	t.Run("summarize with empty args still reads stdin", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := run(nil, strings.NewReader(""), &out, &errBuf)
		assert.Equalf(t, 0, code, "an empty run is not a failed one, stderr: %q", errBuf.String())
	})

	t.Run("summarize failure prints to stderr and exits 1", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := run([]string{"-nonexistent-flag"}, strings.NewReader(""), &out, &errBuf)
		assert.Equal(t, 1, code, "a flag that does not exist must exit non-zero")
		assert.Contains(t, errBuf.String(), "testreport:", "the error must name the tool")
	})
}

func TestRunRender(t *testing.T) {
	t.Run("bad flags surface", func(t *testing.T) {
		assert.Error(t, runRender([]string{"-nonexistent-flag"}, io.Discard), "a flag that does not exist must be refused")
	})

	t.Run("no report paths is a usage error", func(t *testing.T) {
		assert.Error(t, runRender(nil, io.Discard), "there is nothing to render without a report to render")
	})

	t.Run("a missing file surfaces", func(t *testing.T) {
		assert.Error(t, runRender([]string{filepath.Join(t.TempDir(), "nope.json")}, io.Discard),
			"a report that is not there must be reported, not rendered as an empty run")
	})

	t.Run("undecodable JSON surfaces", func(t *testing.T) {
		path := writeFile(t, "bad.json", "not json")
		assert.Error(t, runRender([]string{path}, io.Discard), "a report that could not be decoded must be reported")
	})

	t.Run("a close error surfaces", func(t *testing.T) {
		withOpenFile(t, func(string) (io.ReadCloser, error) {
			return badCloser{Reader: strings.NewReader(`{"os":"linux"}`), closeErr: errors.New("close boom")}, nil
		})
		err := runRender([]string{"any.json"}, io.Discard)
		require.Error(t, err, "a file that could not be closed must be reported")
		assert.ErrorContains(t, err, "close boom", "the error must carry what the close said")
	})

	t.Run("an output write failure surfaces", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		assert.Error(t, runRender([]string{path}, errWriter{}), "a comment that could not be written must be reported")
	})

	t.Run("per-OS artifact URLs parse and render", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		var out bytes.Buffer
		err := runRender([]string{"-report-url", "linux=https://example/r", "-coverage-url", "linux=https://example/c", path}, &out)
		require.NoError(t, err, "runRender")
		assert.Contains(t, out.String(), "https://example/r", "the test-report cell must link to the URL it was given")
		assert.Contains(t, out.String(), "https://example/c", "the coverage cell must link to the URL it was given")
	})
}

func TestRunBadge(t *testing.T) {
	t.Run("wrong argument count is a usage error", func(t *testing.T) {
		assert.Error(t, runBadge(nil, io.Discard), "there is nothing to badge without a report")
	})

	t.Run("a missing file surfaces", func(t *testing.T) {
		assert.Error(t, runBadge([]string{filepath.Join(t.TempDir(), "nope.json")}, io.Discard),
			"a report that is not there must be reported")
	})

	t.Run("undecodable JSON surfaces", func(t *testing.T) {
		path := writeFile(t, "bad.json", "not json")
		assert.Error(t, runBadge([]string{path}, io.Discard), "a report that could not be decoded must be reported")
	})

	t.Run("a report with no coverage surfaces", func(t *testing.T) {
		path := writeJSON(t, Report{OS: "linux"})
		assert.Error(t, runBadge([]string{path}, io.Discard),
			"a run that measured no coverage must not be given a badge claiming a number")
	})

	t.Run("a close error surfaces", func(t *testing.T) {
		withOpenFile(t, func(string) (io.ReadCloser, error) {
			return badCloser{Reader: strings.NewReader(`{"os":"linux","package_coverage":[{"package":"p","percent":50}]}`), closeErr: errors.New("close boom")}, nil
		})
		err := runBadge([]string{"any.json"}, io.Discard)
		require.Error(t, err, "a file that could not be closed must be reported")
		assert.ErrorContains(t, err, "close boom", "the error must carry what the close said")
	})

	t.Run("success writes the badge JSON", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		var out bytes.Buffer
		require.NoError(t, runBadge([]string{path}, &out), "runBadge")
		assert.Contains(t, out.String(), "schemaVersion", "the badge must be the shape shields.io reads")
	})

	t.Run("an output write failure surfaces", func(t *testing.T) {
		path := writeJSON(t, coveredReport())
		assert.Error(t, runBadge([]string{path}, errWriter{}), "a badge that could not be written must be reported")
	})
}

func TestRunSummarize(t *testing.T) {
	validStream := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestFoo"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"pkg","Test":"TestFoo","Elapsed":1.0}`,
	)

	t.Run("bad flags surface", func(t *testing.T) {
		assert.Error(t, runSummarize([]string{"-nonexistent-flag"}, strings.NewReader(""), io.Discard),
			"a flag that does not exist must be refused")
	})

	t.Run("an unparsable event stream surfaces", func(t *testing.T) {
		assert.Error(t, runSummarize(nil, strings.NewReader("not json\n"), io.Discard),
			"a stream that could not be read must be reported, not summarised as an empty run")
	})

	t.Run("a missing coverage profile surfaces", func(t *testing.T) {
		err := runSummarize([]string{"-coverprofile", filepath.Join(t.TempDir(), "nope.cover")}, strings.NewReader(validStream), io.Discard)
		assert.Error(t, err, "a profile that was asked for and is not there must be reported")
	})

	t.Run("a malformed coverage profile surfaces", func(t *testing.T) {
		profile := writeFile(t, "bad.cover", "mode: set\nfile.go:1.1,2.2 notanumber 1\n")
		err := runSummarize([]string{"-coverprofile", profile}, strings.NewReader(validStream), io.Discard)
		assert.Error(t, err, "a profile that could not be read must be reported, not counted as zero")
	})

	t.Run("a coverage-profile close error surfaces", func(t *testing.T) {
		withOpenFile(t, func(string) (io.ReadCloser, error) {
			return badCloser{Reader: strings.NewReader("mode: set\n"), closeErr: errors.New("close boom")}, nil
		})
		err := runSummarize([]string{"-coverprofile", "any.cover"}, strings.NewReader(validStream), io.Discard)
		require.Error(t, err, "a file that could not be closed must be reported")
		assert.ErrorContains(t, err, "close boom", "the error must carry what the close said")
	})

	t.Run("success folds in coverage", func(t *testing.T) {
		profile := writeFile(t, "ok.cover", "mode: set\nsshakku/pkg/x.go:1.1,2.2 4 1\n")
		var out bytes.Buffer
		require.NoError(t, runSummarize([]string{"-os", "linux", "-coverprofile", profile}, strings.NewReader(validStream), &out), "runSummarize")
		assert.Contains(t, out.String(), `"coverage_percent"`, "a profile that was read must reach the report")
	})

	t.Run("success without a coverage profile", func(t *testing.T) {
		var out bytes.Buffer
		require.NoError(t, runSummarize([]string{"-os", "linux"}, strings.NewReader(validStream), &out), "runSummarize")
		assert.Contains(t, out.String(), `"os": "linux"`, "the summary must say which OS it is from")
	})

	t.Run("an output write failure surfaces", func(t *testing.T) {
		assert.Error(t, runSummarize([]string{"-os", "linux"}, strings.NewReader(validStream), errWriter{}),
			"a summary that could not be written must be reported")
	})
}
