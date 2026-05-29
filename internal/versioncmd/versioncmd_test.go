package versioncmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestVersionJSONIncludesCapabilities(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Version      string `json:"version"`
			Capabilities struct {
				SchemaVersion int `json:"schema_version"`
				Commands      []struct {
					Command string `json:"command"`
				} `json:"commands"`
				ConfigFields []string `json:"config_fields"`
				SchemaHashes struct {
					CommandSurface string `json:"command_surface"`
					ConfigFields   string `json:"config_fields"`
				} `json:"schema_hashes"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !payload.OK || payload.Data.Capabilities.SchemaVersion != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	if len(payload.Data.Capabilities.Commands) == 0 {
		t.Fatalf("commands missing: %s", stdout.String())
	}
	if payload.Data.Capabilities.SchemaHashes.CommandSurface == "" || payload.Data.Capabilities.SchemaHashes.ConfigFields == "" {
		t.Fatalf("schema hashes missing: %s", stdout.String())
	}
}
