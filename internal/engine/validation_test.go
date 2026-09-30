package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNamespaceGrammar(t *testing.T) {
	for _, s := range []string{"a", "a.b-c_d9", strings.Repeat("a", 63), strings.Repeat("a.", 7) + "a", strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63)} {
		if !ValidNamespace(s) {
			t.Fatalf("rejected canonical namespace %q", s)
		}
	}
	for _, s := range []string{"", "A", "a..b", ".a", "a.", "1a", "a.*", "*", "a/b", "a b", "a\x00b", "é", strings.Repeat("a", 64), strings.Repeat("a.", 8) + "a", strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 62) + ".e"} {
		if ValidNamespace(s) {
			t.Fatalf("accepted noncanonical namespace %q", s)
		}
	}
}
func TestStrictJSON(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":{"x":1,"\u0078":2}}`, `{} {}`, `{"x":1e309}`, `{"x":1e-309}`, `{"x":"\ud800"}`, `{"x":"\udc00"}`, "{\"x\":\"\xff\"}", strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34), "[" + strings.Repeat("0,", 4096) + "0]", `{"x":` + strings.Repeat("1", 129) + `}`} {
		if _, err := ParseJSON([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid/ambiguous JSON %.100q", raw)
		}
	}
	for _, raw := range []string{`{"x":9007199254740993}`, `{"x":1e308}`, `{"x":"\ud83d\ude00"}`, `{"x":"\\ud800"}`, `{"x":"�"}`} {
		if _, err := ParseJSON([]byte(raw)); err != nil {
			t.Fatalf("rejected valid JSON %q: %v", raw, err)
		}
	}
}
func TestSchemaProfileAndExactNumericValidation(t *testing.T) {
	for _, fragment := range []string{`"$ref":"https://example.com/schema"`, `"$ref":"file:///etc/passwd"`, `"$ref":"#"`, `"$id":"https://example.com"`, `"default":{}`, `"format":"email"`, `"pattern":".*"`, `"oneOf":[{}]`, `"unknown":true`, `"properties":{"x":{"$schema":"https://json-schema.org/draft/2020-12/schema"}}`, `"required":[1]`, `"additionalProperties":{}`, `"maxLength":9223372036854775808`, `"minItems":1e308`, `"maxProperties":-1`} {
		raw := json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object",` + fragment + `}`)
		if _, err := compileSchema(raw); err == nil {
			t.Fatalf("accepted unsupported schema %s", fragment)
		}
	}
	schema := json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"n":{"type":"integer","const":9007199254740993},"decimal":{"type":"number","multipleOf":0.1}},"required":["n","decimal"]}`)
	if err := validateData(schema, json.RawMessage(`{"n":9007199254740993,"decimal":0.3,"unknown":{"kept":true}}`)); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{`{"n":9007199254740992,"decimal":0.3}`, `{"n":9007199254740993,"decimal":0.31}`, `{"n":"9007199254740993","decimal":0.3}`} {
		wantError(t, validateData(schema, json.RawMessage(data)), ErrValidation)
	}
	literals := json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"$ref":{"const":{"$ref":"literal"}}}}`)
	if err := validateData(literals, json.RawMessage(`{"$ref":{"$ref":"literal"}}`)); err != nil {
		t.Fatal("literal property/const was treated as a reference", err)
	}
	if _, err := compileSchema(json.RawMessage(`{"type":"object"}`)); err == nil {
		t.Fatal("missing dialect accepted")
	}
	if err := validateData(json.RawMessage(skillSchema), json.RawMessage(`{"name":"Example","level":2,"extra":true}`)); err == nil {
		t.Fatal("unknown data field silently accepted")
	}
	if err := validateData(json.RawMessage(skillSchema), json.RawMessage(`{"name":"`+strings.Repeat("x", 64<<10)+`","level":2}`)); err == nil {
		t.Fatal("oversize data accepted")
	}
}
