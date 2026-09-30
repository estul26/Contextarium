package engine

import (
	"encoding/json"
	"math/big"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const dialect = "https://json-schema.org/draft/2020-12/schema"

type denyLoader struct{}

func (denyLoader) Load(string) (any, error) { return nil, ErrValidation }

func compileSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	if len(raw) > 32<<10 {
		return nil, ErrValidation
	}
	value, err := ParseJSON(raw)
	if err != nil {
		return nil, ErrValidation
	}
	root, ok := value.(map[string]any)
	if !ok || root["$schema"] != dialect || root["type"] != "object" {
		return nil, ErrValidation
	}
	if err := checkSchema(value, true); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(denyLoader{})
	const resource = "urn:contextarium:m1:schema"
	if err := compiler.AddResource(resource, value); err != nil {
		return nil, ErrValidation
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, ErrValidation
	}
	return compiled, nil
}

func checkSchema(value any, root bool) error {
	if _, ok := value.(bool); ok {
		return nil
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return ErrValidation
	}
	for name, v := range obj {
		switch name {
		case "$schema":
			if !root || v != dialect {
				return ErrValidation
			}
		case "properties":
			properties, ok := v.(map[string]any)
			if !ok {
				return ErrValidation
			}
			for _, child := range properties {
				if err := checkSchema(child, false); err != nil {
					return err
				}
			}
		case "items":
			if err := checkSchema(v, false); err != nil {
				return err
			}
		case "additionalProperties":
			if _, ok := v.(bool); !ok {
				return ErrValidation
			}
		case "minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties":
			n, ok := v.(json.Number)
			if !ok {
				return ErrValidation
			}
			rat, ok := new(big.Rat).SetString(n.String())
			if !ok || !rat.IsInt() || !rat.Num().IsInt64() || rat.Sign() < 0 || rat.Num().Int64() > 2147483647 {
				return ErrValidation
			}
		case "title", "description", "type", "required", "enum", "const", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf":
			// The pinned 2020-12 metaschema validates keyword values.
		default:
			return ErrValidation
		}
	}
	return nil
}

func validateData(schema json.RawMessage, data json.RawMessage) error {
	if len(data) > 64<<10 {
		return ErrInvalid
	}
	value, err := ParseJSON(data)
	if err != nil {
		return err
	}
	if _, ok := value.(map[string]any); !ok {
		return ErrInvalid
	}
	compiled, err := compileSchema(schema)
	if err != nil {
		return err
	}
	if err := compiled.Validate(value); err != nil {
		return ErrValidation
	}
	return nil
}
