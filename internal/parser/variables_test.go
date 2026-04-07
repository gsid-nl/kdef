package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVariableFile(t *testing.T) {
	src := `
variable "environment" {
  type    = "enum[staging, production]"
  default = "staging"
}

variable "image_tag" {
  type    = "string"
  default = "latest"
}

variable "replicas" {
  type    = "number"
  default = 3
}

variable "debug" {
  type    = "bool"
  default = false
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	vars, diags := ParseVariableFile(path)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	// Check environment (enum)
	env, ok := vars["environment"]
	if !ok {
		t.Fatal("missing variable 'environment'")
	}
	if env.Type != "enum[staging, production]" {
		t.Errorf("expected type 'enum[staging, production]', got %q", env.Type)
	}
	if len(env.AllowedValues) != 2 {
		t.Errorf("expected 2 allowed values, got %d", len(env.AllowedValues))
	}
	if env.Default == nil {
		t.Fatal("expected default for environment")
	}
	if env.Default.AsString() != "staging" {
		t.Errorf("expected default 'staging', got %q", env.Default.AsString())
	}
	if env.Required {
		t.Error("environment should not be required (has default)")
	}

	// Check image_tag
	tag, ok := vars["image_tag"]
	if !ok {
		t.Fatal("missing variable 'image_tag'")
	}
	if tag.Type != "string" {
		t.Errorf("expected type 'string', got %q", tag.Type)
	}
	if tag.Default.AsString() != "latest" {
		t.Errorf("expected default 'latest', got %q", tag.Default.AsString())
	}

	// Check replicas (number)
	rep, ok := vars["replicas"]
	if !ok {
		t.Fatal("missing variable 'replicas'")
	}
	if rep.Type != "number" {
		t.Errorf("expected type 'number', got %q", rep.Type)
	}

	// Check debug (bool)
	dbg, ok := vars["debug"]
	if !ok {
		t.Fatal("missing variable 'debug'")
	}
	if dbg.Type != "bool" {
		t.Errorf("expected type 'bool', got %q", dbg.Type)
	}
}

func TestParseVariableRequired(t *testing.T) {
	src := `
variable "api_key" {
  type = "string"
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	vars, diags := ParseVariableFile(path)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	apiKey := vars["api_key"]
	if !apiKey.Required {
		t.Error("api_key should be required (no default)")
	}
}

func TestBuildEvalContext(t *testing.T) {
	src := `
variable "environment" {
  type    = "string"
  default = "staging"
}

variable "image_tag" {
  type    = "string"
  default = "latest"
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	vars, diags := ParseVariableFile(path)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %s", diags.Error())
	}

	// Test with override
	overrides := map[string]string{"image_tag": "v2.0.0"}
	ctx, diags := BuildEvalContext(vars, overrides, nil)
	if diags.HasErrors() {
		t.Fatalf("context errors: %s", diags.Error())
	}

	varObj := ctx.Variables["var"]
	tagVal := varObj.GetAttr("image_tag")
	if tagVal.AsString() != "v2.0.0" {
		t.Errorf("expected override 'v2.0.0', got %q", tagVal.AsString())
	}

	envVal := varObj.GetAttr("environment")
	if envVal.AsString() != "staging" {
		t.Errorf("expected default 'staging', got %q", envVal.AsString())
	}
}

func TestBuildEvalContextMissingRequired(t *testing.T) {
	src := `
variable "api_key" {
  type = "string"
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	vars, diags := ParseVariableFile(path)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %s", diags.Error())
	}

	_, diags = BuildEvalContext(vars, nil, nil)
	if !diags.HasErrors() {
		t.Error("expected error for missing required variable")
	}
}
