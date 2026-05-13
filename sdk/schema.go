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

		ft := f.Type
		// Dereference pointers.
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		// Handle embedded structs by flattening their fields.
		if f.Anonymous && ft.Kind() == reflect.Struct {
			nested := extractSchema(ft)
			fields = append(fields, nested.Fields...)

			continue
		}

		// Recurse into nested structs.
		if ft.Kind() == reflect.Struct {
			nested := extractSchema(ft)

			prefix := JSONFieldName(jsonTag, f.Name)
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
			JSONName:    JSONFieldName(jsonTag, f.Name),
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

// JSONFieldName extracts the JSON field name from a json struct tag.
func JSONFieldName(tag, fallback string) string {
	if tag == "" {
		return fallback
	}

	if before, _, ok := strings.Cut(tag, ","); ok {
		return before
	}

	return tag
}
