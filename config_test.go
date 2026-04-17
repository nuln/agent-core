package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearConfigSpecs() {
	configSpecMu.Lock()
	defer configSpecMu.Unlock()
	configSpecsMap = map[string]PluginConfigSpec{}
}

func TestRegisterAndGetPluginConfigSpec(t *testing.T) {
	clearConfigSpecs()
	defer clearConfigSpecs()

	spec := PluginConfigSpec{
		PluginName: "testplugin",
		PluginType: "pipe",
		Fields: []ConfigField{
			{EnvVar: "TP_TOKEN", Key: "token", Required: true, Type: ConfigFieldSecret},
		},
	}
	RegisterPluginConfigSpec(spec)

	got, ok := GetPluginConfigSpec("testplugin")
	if !ok {
		t.Fatal("expected to find registered spec")
	}
	if got.PluginName != "testplugin" {
		t.Fatalf("got name %q, want testplugin", got.PluginName)
	}
	if len(got.Fields) != 1 || got.Fields[0].EnvVar != "TP_TOKEN" {
		t.Fatal("field mismatch")
	}

	_, ok = GetPluginConfigSpec("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent spec")
	}
}

func TestListPluginConfigSpecs_Sorted(t *testing.T) {
	clearConfigSpecs()
	defer clearConfigSpecs()

	for _, name := range []string{"zebra", "alpha", "middle"} {
		RegisterPluginConfigSpec(PluginConfigSpec{PluginName: name})
	}
	specs := ListPluginConfigSpecs()
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}
	if specs[0].PluginName != "alpha" || specs[1].PluginName != "middle" || specs[2].PluginName != "zebra" {
		t.Fatalf("not sorted: %v %v %v", specs[0].PluginName, specs[1].PluginName, specs[2].PluginName)
	}
}

func TestResolveConfigValue_Priority(t *testing.T) {
	field := ConfigField{
		EnvVar:  "TEST_RESOLVE_VAR",
		Key:     "resolve_key",
		Default: "default_val",
	}

	// 1. Default only
	_ = os.Unsetenv("TEST_RESOLVE_VAR")
	v, ok := ResolveConfigValue(field, nil)
	if !ok || v != "default_val" {
		t.Fatalf("expected default_val, got %q ok=%v", v, ok)
	}

	// 2. opts overrides default
	v, ok = ResolveConfigValue(field, map[string]any{"resolve_key": "from_opts"})
	if !ok || v != "from_opts" {
		t.Fatalf("expected from_opts, got %q ok=%v", v, ok)
	}

	// 3. env overrides opts
	_ = os.Setenv("TEST_RESOLVE_VAR", "from_env")
	defer func() { _ = os.Unsetenv("TEST_RESOLVE_VAR") }()
	v, ok = ResolveConfigValue(field, map[string]any{"resolve_key": "from_opts"})
	if !ok || v != "from_env" {
		t.Fatalf("expected from_env, got %q ok=%v", v, ok)
	}
}

func TestResolveConfigValue_NoSources(t *testing.T) {
	field := ConfigField{EnvVar: "TEST_EMPTY_RESOLVE", Key: "empty_key"}
	_ = os.Unsetenv("TEST_EMPTY_RESOLVE")
	_, ok := ResolveConfigValue(field, nil)
	if ok {
		t.Fatal("expected not found when no sources set")
	}
}

func TestResolveAllConfig(t *testing.T) {
	clearConfigSpecs()
	defer clearConfigSpecs()

	spec := PluginConfigSpec{
		PluginName: "all",
		Fields: []ConfigField{
			{EnvVar: "TEST_ALL_A", Key: "a", Default: "da"},
			{EnvVar: "TEST_ALL_B", Key: "b"},
		},
	}
	_ = os.Unsetenv("TEST_ALL_A")
	_ = os.Unsetenv("TEST_ALL_B")

	result := ResolveAllConfig(spec, map[string]any{"b": "bval"})
	if result["a"] != "da" {
		t.Fatalf("expected a=da, got %q", result["a"])
	}
	if result["b"] != "bval" {
		t.Fatalf("expected b=bval, got %q", result["b"])
	}
}

func TestAutoLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := `# comment
KEY_ONE=value1
KEY_TWO="quoted value"
KEY_THREE='single quoted'
EMPTY=
NOEQ_LINE
`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Ensure these are unset
	_ = os.Unsetenv("KEY_ONE")
	_ = os.Unsetenv("KEY_TWO")
	_ = os.Unsetenv("KEY_THREE")
	_ = os.Unsetenv("EMPTY")
	defer func() {
		_ = os.Unsetenv("KEY_ONE")
		_ = os.Unsetenv("KEY_TWO")
		_ = os.Unsetenv("KEY_THREE")
	}()

	if err := AutoLoadEnvFile(envFile); err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv("KEY_ONE"); v != "value1" {
		t.Fatalf("KEY_ONE=%q, want value1", v)
	}
	if v := os.Getenv("KEY_TWO"); v != "quoted value" {
		t.Fatalf("KEY_TWO=%q, want 'quoted value'", v)
	}
	if v := os.Getenv("KEY_THREE"); v != "single quoted" {
		t.Fatalf("KEY_THREE=%q, want 'single quoted'", v)
	}

	// Test no-overwrite: set KEY_ONE and reload
	_ = os.Setenv("KEY_ONE", "original")
	if err := AutoLoadEnvFile(envFile); err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv("KEY_ONE"); v != "original" {
		t.Fatalf("KEY_ONE should not be overwritten, got %q", v)
	}
}

func TestAutoLoadEnvFile_FileNotExist(t *testing.T) {
	err := AutoLoadEnvFile("/nonexistent/.env.nope")
	if err != nil {
		t.Fatal("expected nil for non-existent file")
	}
}

func TestValidatePluginConfig(t *testing.T) {
	clearConfigSpecs()
	defer clearConfigSpecs()

	RegisterPluginConfigSpec(PluginConfigSpec{
		PluginName: "v",
		Fields: []ConfigField{
			{EnvVar: "V_REQUIRED", Key: "req", Required: true, Description: "required field"},
			{EnvVar: "V_OPTIONAL", Key: "opt", Required: false, Default: "x"},
		},
	})

	_ = os.Unsetenv("V_REQUIRED")
	_ = os.Unsetenv("V_OPTIONAL")

	errs := ValidatePluginConfig("v", nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].FieldEnvVar != "V_REQUIRED" {
		t.Fatalf("wrong field: %s", errs[0].FieldEnvVar)
	}

	// Satisfy via opts
	errs = ValidatePluginConfig("v", map[string]any{"req": "ok"})
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(errs))
	}

	// Satisfy via env
	_ = os.Setenv("V_REQUIRED", "yes")
	defer func() { _ = os.Unsetenv("V_REQUIRED") }()
	errs = ValidatePluginConfig("v", nil)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors with env set, got %d", len(errs))
	}
}

func TestValidateAllPluginConfigs(t *testing.T) {
	clearConfigSpecs()
	defer clearConfigSpecs()

	RegisterPluginConfigSpec(PluginConfigSpec{
		PluginName: "p1",
		Fields:     []ConfigField{{EnvVar: "P1_KEY", Key: "key", Required: true}},
	})
	RegisterPluginConfigSpec(PluginConfigSpec{
		PluginName: "p2",
		Fields:     []ConfigField{{EnvVar: "P2_KEY", Key: "key", Required: true}},
	})

	_ = os.Unsetenv("P1_KEY")
	_ = os.Unsetenv("P2_KEY")

	errs := ValidateAllPluginConfigs(nil)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}

	errs = ValidateAllPluginConfigs(map[string]map[string]any{
		"p1": {"key": "val"},
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error (p2 only), got %d", len(errs))
	}
}

func TestGenerateEnvTemplate(t *testing.T) {
	clearConfigSpecs()
	defer clearConfigSpecs()

	RegisterPluginConfigSpec(PluginConfigSpec{
		PluginName:  "webhook",
		PluginType:  "pipe",
		Description: "HTTP webhook endpoint",
		Fields: []ConfigField{
			{EnvVar: "WEBHOOK_PORT", Key: "port", Description: "Listen port", Default: "9111", Type: ConfigFieldInt},
			{EnvVar: "WEBHOOK_TOKEN", Key: "token", Description: "Auth token", Type: ConfigFieldSecret, Required: true},
		},
	})

	out := GenerateEnvTemplate()
	if !strings.Contains(out, "WEBHOOK_PORT") {
		t.Fatal("missing WEBHOOK_PORT in template")
	}
	if !strings.Contains(out, "WEBHOOK_TOKEN") {
		t.Fatal("missing WEBHOOK_TOKEN in template")
	}
	if !strings.Contains(out, "[required]") {
		t.Fatal("missing [required] tag")
	}
	if !strings.Contains(out, "[optional]") {
		t.Fatal("missing [optional] tag")
	}
}

func TestGenerateEnvTemplate_SelectedPlugins(t *testing.T) {
	clearConfigSpecs()
	defer clearConfigSpecs()

	RegisterPluginConfigSpec(PluginConfigSpec{
		PluginName: "a",
		Fields:     []ConfigField{{EnvVar: "A_VAR"}},
	})
	RegisterPluginConfigSpec(PluginConfigSpec{
		PluginName: "b",
		Fields:     []ConfigField{{EnvVar: "B_VAR"}},
	})

	out := GenerateEnvTemplate("a")
	if !strings.Contains(out, "A_VAR") {
		t.Fatal("should contain A_VAR")
	}
	if strings.Contains(out, "B_VAR") {
		t.Fatal("should NOT contain B_VAR")
	}
}

func TestConfigValidationError_Error(t *testing.T) {
	e1 := ConfigValidationError{PluginName: "p", FieldEnvVar: "VAR", Description: "desc"}
	if !strings.Contains(e1.Error(), "VAR") {
		t.Fatal("error string should mention env var")
	}
	e2 := ConfigValidationError{PluginName: "p", FieldKey: "k", Description: "desc"}
	if !strings.Contains(e2.Error(), "k") {
		t.Fatal("error string should mention key")
	}
}
