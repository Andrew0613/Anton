package adapter

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func readYAMLFile[T any](path string, target *T) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(content, target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func marshalYAML(value any) ([]byte, error) {
	content, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func writeYAMLFile(path string, value any) error {
	content, err := marshalYAML(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func trimString(value string) string {
	return strings.TrimSpace(value)
}
