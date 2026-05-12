package sdk

import (
	"reflect"
	"strings"
)

// SchemaField describes a single field in an extension's configuration struct.
type SchemaField struct {
	Name        string // Go field name
	JSONName    string // JSON key (used for settings file and help flags)
	Type        string // Go type string
	Default     string // Value from `default` tag
	Description string // Value from `description` tag
	Env         string // Value from `env` tag
	Flag        string // Value from `flag` tag
	Short       string // Value from `short` tag
	Validate    string // Value from `validate` tag
}

// Schema holds the extracted configuration schema for a registered extension.
type Schema struct {
	Fields []SchemaField
}

// extractSchema reflects on a struct type and extracts its configuration schema.
// It returns an empty Schema for non-struct types (e.g., struct{}).
func extractSchema(t reflect.Type) Schema {
	if t == nil {
		return Schema{}
	}

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return Schema{}
	}

	var fields []SchemaField

	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}

		jsonTag := f.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		// Recurse into nested structs.
		if f.Type.Kind() == reflect.Struct {
			nested := extractSchema(f.Type)

			prefix := jsonFieldName(jsonTag, f.Name)
			for _, nf := range nested.Fields {
				fields = append(fields, SchemaField{
					Name:        f.Name + "." + nf.Name,
					JSONName:    prefix + "." + nf.JSONName,
					Type:        nf.Type,
					Default:     nf.Default,
					Description: nf.Description,
					Env:         nf.Env,
					Flag:        nf.Flag,
					Short:       nf.Short,
					Validate:    nf.Validate,
				})
			}

			continue
		}

		fields = append(fields, SchemaField{
			Name:        f.Name,
			JSONName:    jsonFieldName(jsonTag, f.Name),
			Type:        f.Type.String(),
			Default:     f.Tag.Get("default"),
			Description: f.Tag.Get("description"),
			Env:         f.Tag.Get("env"),
			Flag:        f.Tag.Get("flag"),
			Short:       f.Tag.Get("short"),
			Validate:    f.Tag.Get("validate"),
		})
	}

	return Schema{Fields: fields}
}

// jsonFieldName extracts the JSON field name from a json struct tag.
func jsonFieldName(tag, fallback string) string {
	if tag == "" {
		return fallback
	}

	if before, _, ok := strings.Cut(tag, ","); ok {
		return before
	}

	return tag
}
