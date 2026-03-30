package agent

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// ──────────────────────────────────────────────────────────────
// Config Field Types
// ──────────────────────────────────────────────────────────────

// ConfigFieldType describes the kind of a configuration field.
type ConfigFieldType string

const (
	ConfigFieldString ConfigFieldType = "string"
	ConfigFieldBool   ConfigFieldType = "bool"
	ConfigFieldInt    ConfigFieldType = "int"
	ConfigFieldEnum   ConfigFieldType = "enum"
	ConfigFieldSecret ConfigFieldType = "secret" // masked in templates and logs
)

// ──────────────────────────────────────────────────────────────
// Config Field & Spec
// ──────────────────────────────────────────────────────────────

// ConfigField describes a single configuration item for a plugin.
type ConfigField struct {
	// EnvVar is the environment variable name (e.g. "LARK_APP_ID").
	// Highest priority: if set, always used.
	EnvVar string

	// Key is the opts map key (e.g. "app_id") used in config files.
	Key string

	// Description is a human-readable description.
	Description string

	// Required marks the field as mandatory.
	Required bool

	// Default is the default value (ignored when Required=true).
	Default string

	// Type describes what kind of value the field holds.
	Type ConfigFieldType

	// AllowedValues lists valid values when Type == ConfigFieldEnum.
	AllowedValues []string

	// Example is a sample value for generated templates.
	Example string
}

// PluginConfigSpec describes the full configuration surface of a plugin.
type PluginConfigSpec struct {
	// PluginName matches the registration name (e.g. "lark", "ratelimiter").
	PluginName string

	// PluginType is "dialog", "llm", or "pipe".
	PluginType string

	// Description is a human-readable description of the plugin.
	Description string

	// Fields is the ordered list of configuration fields.
	Fields []ConfigField

	// ExampleTOML is an optional TOML snippet for documentation.
	ExampleTOML string

	// ExampleEnv is an optional .env snippet for documentation.
	ExampleEnv string
}

// ──────────────────────────────────────────────────────────────
// Global Plugin Config Registry
// ──────────────────────────────────────────────────────────────

var (
	configSpecMu   sync.RWMutex
	configSpecsMap = map[string]PluginConfigSpec{}
)

// RegisterPluginConfigSpec records a plugin's config specification.
// Typically called from init() alongside RegisterPipe / RegisterDialog / RegisterLLM.
func RegisterPluginConfigSpec(spec PluginConfigSpec) {
	configSpecMu.Lock()
	defer configSpecMu.Unlock()
	configSpecsMap[spec.PluginName] = spec
}

// GetPluginConfigSpec returns the spec for a single named plugin.
func GetPluginConfigSpec(pluginName string) (PluginConfigSpec, bool) {
	configSpecMu.RLock()
	defer configSpecMu.RUnlock()
	spec, ok := configSpecsMap[pluginName]
	return spec, ok
}

// ListPluginConfigSpecs returns all registered specs in alphabetical order.
func ListPluginConfigSpecs() []PluginConfigSpec {
	configSpecMu.RLock()
	defer configSpecMu.RUnlock()
	specs := make([]PluginConfigSpec, 0, len(configSpecsMap))
	for _, s := range configSpecsMap {
		specs = append(specs, s)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].PluginName < specs[j].PluginName
	})
	return specs
}

// ──────────────────────────────────────────────────────────────
// Config Resolution Helpers
// ──────────────────────────────────────────────────────────────

// ResolveConfigValue resolves the effective value of a field using the
// priority chain: env var → opts map → default.
// Returns (value, found). found is false only when no source provided a value.
func ResolveConfigValue(f ConfigField, opts map[string]any) (string, bool) {
	// 1. Environment variable (highest priority)
	if f.EnvVar != "" {
		if v := os.Getenv(f.EnvVar); v != "" {
			return v, true
		}
	}

	// 2. opts map
	if f.Key != "" && opts != nil {
		if v, ok := opts[f.Key]; ok && v != nil {
			s := fmt.Sprintf("%v", v)
			if s != "" {
				return s, true
			}
		}
	}

	// 3. Default
	if f.Default != "" {
		return f.Default, true
	}

	return "", false
}

// ResolveAllConfig resolves all fields for a plugin spec, returning a map of key → value.
// Fields without EnvVar use their Key; fields without Key use their EnvVar.
func ResolveAllConfig(spec PluginConfigSpec, opts map[string]any) map[string]string {
	result := make(map[string]string, len(spec.Fields))
	for _, f := range spec.Fields {
		key := f.Key
		if key == "" {
			key = f.EnvVar
		}
		if key == "" {
			continue
		}
		if v, found := ResolveConfigValue(f, opts); found {
			result[key] = v
		}
	}
	return result
}

// ──────────────────────────────────────────────────────────────
// Environment File Loader
// ──────────────────────────────────────────────────────────────

// AutoLoadEnvFile loads environment variables from a dotenv-style file.
// Existing env vars are NOT overwritten. Returns nil if file does not exist.
// Lines starting with # are comments; format: KEY=VALUE (quotes stripped).
func AutoLoadEnvFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		// Do not overwrite existing
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

// ──────────────────────────────────────────────────────────────
// Validation
// ──────────────────────────────────────────────────────────────

// ConfigValidationError describes a missing or invalid config field.
type ConfigValidationError struct {
	PluginName  string
	FieldEnvVar string
	FieldKey    string
	Description string
}

func (e ConfigValidationError) Error() string {
	if e.FieldEnvVar != "" {
		return fmt.Sprintf("plugin %q: required env var %s not set (%s)", e.PluginName, e.FieldEnvVar, e.Description)
	}
	return fmt.Sprintf("plugin %q: required config key %q not set (%s)", e.PluginName, e.FieldKey, e.Description)
}

// ValidatePluginConfig checks that all required fields of a named plugin are satisfied.
// opts is the application-level config map (may be nil).
func ValidatePluginConfig(pluginName string, opts map[string]any) []ConfigValidationError {
	spec, ok := GetPluginConfigSpec(pluginName)
	if !ok {
		return nil
	}
	var errs []ConfigValidationError
	for _, f := range spec.Fields {
		if !f.Required {
			continue
		}
		if _, found := ResolveConfigValue(f, opts); !found {
			errs = append(errs, ConfigValidationError{
				PluginName:  pluginName,
				FieldEnvVar: f.EnvVar,
				FieldKey:    f.Key,
				Description: f.Description,
			})
		}
	}
	return errs
}

// ValidateAllPluginConfigs checks all registered plugins.
// optsPerPlugin maps plugin names to their opts (may be nil for env-only plugins).
func ValidateAllPluginConfigs(optsPerPlugin map[string]map[string]any) []ConfigValidationError {
	specs := ListPluginConfigSpecs()
	var errs []ConfigValidationError
	for _, spec := range specs {
		var opts map[string]any
		if optsPerPlugin != nil {
			opts = optsPerPlugin[spec.PluginName]
		}
		errs = append(errs, ValidatePluginConfig(spec.PluginName, opts)...)
	}
	return errs
}

// ──────────────────────────────────────────────────────────────
// Template Generation
// ──────────────────────────────────────────────────────────────

// GenerateEnvTemplate produces a .env template for the specified plugins
// (or all registered plugins if no names given).
func GenerateEnvTemplate(pluginNames ...string) string {
	var sb strings.Builder
	specs := listSpecsForNames(pluginNames)

	sb.WriteString("# Plugin configuration template\n")
	sb.WriteString("# Save as .env and fill in the real values\n\n")

	for _, spec := range specs {
		if !hasEnvFields(spec) {
			continue
		}
		sb.WriteString(fmt.Sprintf("# ── %s (%s) ─────────────────────\n", spec.PluginName, spec.Description))
		for _, f := range spec.Fields {
			if f.EnvVar == "" {
				continue
			}
			tag := " [optional]"
			if f.Required {
				tag = " [required]"
			}
			sb.WriteString(fmt.Sprintf("# %s%s\n", f.Description, tag))
			if len(f.AllowedValues) > 0 {
				sb.WriteString(fmt.Sprintf("# Allowed: %s\n", strings.Join(f.AllowedValues, " | ")))
			}
			placeholder := f.Example
			if placeholder == "" {
				if f.Type == ConfigFieldSecret {
					placeholder = "your-secret-here"
				} else if f.Default != "" {
					placeholder = f.Default
				} else {
					placeholder = "your-value-here"
				}
			}
			if !f.Required && f.Default != "" {
				// Commented-out line for optional fields with defaults
				sb.WriteString(fmt.Sprintf("# %s=%s\n", f.EnvVar, f.Default))
			} else {
				sb.WriteString(fmt.Sprintf("%s=%s\n", f.EnvVar, placeholder))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// GenerateTOMLTemplate produces TOML snippets for the specified plugins.
func GenerateTOMLTemplate(pluginNames ...string) string {
	var sb strings.Builder
	specs := listSpecsForNames(pluginNames)
	for _, spec := range specs {
		if spec.ExampleTOML != "" {
			sb.WriteString(fmt.Sprintf("# ── %s ──\n", spec.PluginName))
			sb.WriteString(spec.ExampleTOML)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// ──────────────────────────────────────────────────────────────
// Internal Helpers
// ──────────────────────────────────────────────────────────────

func listSpecsForNames(names []string) []PluginConfigSpec {
	if len(names) == 0 {
		return ListPluginConfigSpecs() // already sorted
	}
	var result []PluginConfigSpec
	for _, n := range names {
		if s, ok := GetPluginConfigSpec(n); ok {
			result = append(result, s)
		}
	}
	return result
}

func hasEnvFields(spec PluginConfigSpec) bool {
	for _, f := range spec.Fields {
		if f.EnvVar != "" {
			return true
		}
	}
	return false
}
