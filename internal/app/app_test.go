package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if strings.TrimSpace(stdout.String()) != "anton 0.0.1" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTopLevelVersionFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--version"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if strings.TrimSpace(stdout.String()) != "anton 0.0.1" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestVersionCommandJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version", "--json"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if !strings.Contains(stdout.String(), `"version": "0.0.1"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
