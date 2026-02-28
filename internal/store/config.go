package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ConfigKeyType describes the value type for validation.
type ConfigKeyType string

const (
	ConfigString   ConfigKeyType = "string"
	ConfigBool     ConfigKeyType = "bool"
	ConfigInt      ConfigKeyType = "int"
	ConfigDuration ConfigKeyType = "duration"
)

// ConfigKeyDef defines a single config key.
type ConfigKeyDef struct {
	Key          string        // dot-notation CLI key
	JSONField    string        // field name in config.json
	Type         ConfigKeyType // value type
	DefaultValue string        // human-readable default
	Description  string        // short help text
}

// configKeys is the authoritative registry of system config keys.
// Default values for models duplicate pipeline.DefaultClassifierModel and
// pipeline.DefaultEvaluatorModel to avoid a circular import (store ← pipeline).
// The canonical values remain in pipeline/classifier.go and pipeline/evaluator.go.
var configKeys = []ConfigKeyDef{
	{"debug", "debug", ConfigBool, "false", "Enable debug mode (persist CC sessions)"},
	{"classifier.model", "classifierModel", ConfigString, "claude-haiku-4-5", "Claude model for classification"},
	{"evaluator.model", "evaluatorModel", ConfigString, "claude-sonnet-4-6", "Claude model for evaluation"},
	{"classifier.timeout", "classifierTimeout", ConfigDuration, "3m", "Classifier invocation timeout"},
	{"evaluator.timeout", "evaluatorTimeout", ConfigDuration, "7m", "Evaluator invocation timeout"},
	{"circuit-breaker.threshold", "circuitBreakerThreshold", ConfigInt, "5", "Consecutive errors before circuit opens"},
	{"circuit-breaker.cooldown", "circuitBreakerCooldown", ConfigDuration, "30m", "Cooldown before circuit half-opens"},
}

// ConfigKeys returns the full registry (for list/help).
func ConfigKeys() []ConfigKeyDef {
	return configKeys
}

// LookupConfigKey finds a key definition by dot-notation name.
func LookupConfigKey(key string) (ConfigKeyDef, bool) {
	for _, k := range configKeys {
		if k.Key == key {
			return k, true
		}
	}
	return ConfigKeyDef{}, false
}

func configPath() string {
	return filepath.Join(Root(), "config.json")
}

// readRawConfig reads config.json as a raw JSON map.
// Returns empty map if file is missing.
func readRawConfig() (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]json.RawMessage), nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return m, nil
}

// writeRawConfig writes the raw JSON map back atomically.
func writeRawConfig(m map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')
	return AtomicWrite(configPath(), data, 0o644)
}

// ConfigGet returns the effective value of a config key and whether it's the default.
func ConfigGet(key string) (string, bool, error) {
	def, ok := LookupConfigKey(key)
	if !ok {
		return "", false, fmt.Errorf("unknown config key: %q", key)
	}
	m, err := readRawConfig()
	if err != nil {
		return def.DefaultValue, true, nil // fallback to default on read error
	}
	raw, exists := m[def.JSONField]
	if !exists {
		return def.DefaultValue, true, nil
	}
	val, err := rawToString(raw, def.Type)
	if err != nil {
		return def.DefaultValue, true, nil
	}
	return val, false, nil
}

// rawToString converts a json.RawMessage to a display string based on type.
func rawToString(raw json.RawMessage, typ ConfigKeyType) (string, error) {
	switch typ {
	case ConfigString, ConfigDuration:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	case ConfigBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return "", err
		}
		return strconv.FormatBool(b), nil
	case ConfigInt:
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			return "", err
		}
		return strconv.Itoa(n), nil
	default:
		return string(raw), nil
	}
}

// validateValue checks that value is valid for the given key type.
func validateValue(value string, def ConfigKeyDef) error {
	switch def.Type {
	case ConfigBool:
		if value != "true" && value != "false" {
			return fmt.Errorf("invalid bool value %q (must be true or false)", value)
		}
	case ConfigInt:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
	case ConfigDuration:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("invalid duration %q: %w", value, err)
		}
	case ConfigString:
		// no validation
	}
	return nil
}

// valueToRaw converts a validated string value to json.RawMessage.
func valueToRaw(value string, typ ConfigKeyType) (json.RawMessage, error) {
	switch typ {
	case ConfigString, ConfigDuration:
		return json.Marshal(value)
	case ConfigBool:
		b, _ := strconv.ParseBool(value)
		return json.Marshal(b)
	case ConfigInt:
		n, _ := strconv.Atoi(value)
		return json.Marshal(n)
	default:
		return json.Marshal(value)
	}
}

// ConfigSet validates and writes a config key.
func ConfigSet(key, value string) error {
	def, ok := LookupConfigKey(key)
	if !ok {
		return fmt.Errorf("unknown config key: %q", key)
	}
	if err := validateValue(value, def); err != nil {
		return err
	}
	m, err := readRawConfig()
	if err != nil {
		return err
	}
	raw, err := valueToRaw(value, def.Type)
	if err != nil {
		return fmt.Errorf("encoding value: %w", err)
	}
	m[def.JSONField] = raw
	return writeRawConfig(m)
}

// ConfigUnset removes a config key, reverting it to its default.
func ConfigUnset(key string) error {
	def, ok := LookupConfigKey(key)
	if !ok {
		return fmt.Errorf("unknown config key: %q", key)
	}
	m, err := readRawConfig()
	if err != nil {
		return err
	}
	if _, exists := m[def.JSONField]; !exists {
		return nil // already absent
	}
	delete(m, def.JSONField)
	return writeRawConfig(m)
}

// ConfigEntry is a key with its effective value and source.
type ConfigEntry struct {
	Key       string
	Value     string
	IsDefault bool
}

// ConfigList returns all system config keys with their effective values.
func ConfigList() ([]ConfigEntry, error) {
	entries := make([]ConfigEntry, 0, len(configKeys))
	for _, def := range configKeys {
		val, isDefault, err := ConfigGet(def.Key)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ConfigEntry{
			Key:       def.Key,
			Value:     val,
			IsDefault: isDefault,
		})
	}
	return entries, nil
}
