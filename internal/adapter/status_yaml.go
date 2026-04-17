package adapter

import (
	"bytes"
	"fmt"
	"io"
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

func readYAMLFileStrict[T any](path string, target *T) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	var extraDoc any
	err = decoder.Decode(&extraDoc)
	if err == nil {
		return fmt.Errorf("parse %s: multiple YAML documents are not supported", path)
	}
	if err != io.EOF {
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
