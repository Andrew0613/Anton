package gates

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseValidGateDeclarations(t *testing.T) {
	set, err := LoadFile("testdata/config/success.yaml", "explicit config")
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if !set.OK() {
		t.Fatalf("set should be ok: %#v", set.Findings)
	}
	if set.Summary.Declared != 2 || set.Summary.Required != 1 {
		t.Fatalf("summary = %#v", set.Summary)
	}
	if got := set.Gates[0].Command.Argv[0]; got != "go" {
		t.Fatalf("first argv = %q", got)
	}
}

func TestCheckMissingRequiredCommandWarns(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--json", "--config", "testdata/config/missing_required.yaml"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertGoldenJSON(t, stdout.Bytes(), "gates_check_missing_required.json")
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestUnsafeCommandContentIsInertWarning(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "should-not-exist")
	content := strings.ReplaceAll(readTestFile(t, "testdata/config/unsafe_inert.yaml"), "__MARKER__", marker)
	configPath := filepath.Join(t.TempDir(), "anton.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"check", "--json", "--config", configPath}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("gates check must not execute command metadata; marker stat err=%v", err)
	}

	normalized := bytes.ReplaceAll(stdout.Bytes(), []byte(configPath), []byte("testdata/config/unsafe_inert.yaml"))
	normalized = bytes.ReplaceAll(normalized, []byte(marker), []byte("testdata/config/unsafe_inert.yaml"))
	normalized = bytes.ReplaceAll(normalized, []byte(`\u003e`), []byte(`>`))
	assertGoldenJSON(t, normalized, "gates_check_unsafe_inert.json")
}

func TestListEmptyJSONContract(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"list", "--json", "--config", "testdata/config/empty.yaml"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertGoldenJSON(t, stdout.Bytes(), "gates_list_empty.json")
}

func TestListSuccessJSONContract(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"list", "--json", "--config", "testdata/config/success.yaml"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertGoldenJSON(t, stdout.Bytes(), "gates_list_success.json")
}

func TestCheckSuccessJSONContract(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--json", "--config", "testdata/config/success.yaml"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertGoldenJSON(t, stdout.Bytes(), "gates_check_success.json")
}

func TestCheckMalformedJSONContract(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--json", "--config", "testdata/config/malformed.yaml"}, &stdout, &stderr, nil)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertGoldenJSON(t, stdout.Bytes(), "gates_check_malformed.json")
}

func TestUsageErrorJSONContract(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--json", "--bad"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertGoldenJSON(t, stdout.Bytes(), "gates_usage_error.json")
}

func TestRunRemainsNotApproved(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"run", "--json", "--config", "testdata/config/success.yaml"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	payload := struct {
		Error *errorPayload `json:"error"`
	}{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if payload.Error == nil || payload.Error.Code != "not-approved" {
		t.Fatalf("error = %+v", payload.Error)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func assertGoldenJSON(t *testing.T, actual []byte, name string) {
	t.Helper()
	actual = bytes.ReplaceAll(actual, []byte(`\u003e`), []byte(`>`))
	var actualValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("actual is not json: %v\n%s", err, string(actual))
	}
	normalized, err := json.MarshalIndent(actualValue, "", "  ")
	if err != nil {
		t.Fatalf("marshal actual: %v", err)
	}
	normalized = append(normalized, '\n')

	expected := []byte(readTestFile(t, filepath.Join("testdata/golden", name)))
	if !bytes.Equal(normalized, expected) {
		t.Fatalf("json mismatch for %s\nactual:\n%s\nexpected:\n%s", name, string(normalized), string(expected))
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
