package settings

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Loader loads configuration into a target struct from multiple sources.
// Priority order: defaults → JSON data → env vars → CLI flags → validation.
type Loader struct {
	Data      map[string]any // JSON subtree to load from
	EnvPrefix string         // Prefix for env vars (e.g. "WEAVE_BASH")
	Args      []string       // CLI args for flag parsing
}

// Load populates the target struct using data, env, flags, defaults, and validation.
// The target must be a non-nil pointer to a struct.
func (l *Loader) Load(target any) error {
	if target == nil {
		return errors.New("loader target is nil")
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("loader target must be a non-nil pointer, got %T", target)
	}

	if v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("loader target must point to a struct, got %T", target)
	}

	// Priority: defaults → data → env → flags → validation.
	if err := applyDefaults(target); err != nil {
		return fmt.Errorf("apply defaults: %w", err)
	}

	if l.Data != nil {
		if err := applyData(target, l.Data); err != nil {
			return fmt.Errorf("apply data: %w", err)
		}
	}

	if l.EnvPrefix != "" {
		if err := applyEnv(target, l.EnvPrefix); err != nil {
			return fmt.Errorf("apply env: %w", err)
		}
	}

	if len(l.Args) > 0 {
		if _, err := applyFlags(target, l.Args); err != nil {
			return fmt.Errorf("apply flags: %w", err)
		}
	}

	if err := validate(target); err != nil {
		return err
	}

	return nil
}

// applyDefaults sets zero-value fields from their `default` struct tags.
func applyDefaults(target any) error {
	if target == nil {
		return nil
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		ft := t.Field(i)

		// Recurse into nested structs.
		if field.Kind() == reflect.Struct {
			if err := applyDefaults(field.Addr().Interface()); err != nil {
				return err
			}

			continue
		}

		defTag := ft.Tag.Get("default")
		if defTag == "" {
			continue
		}

		if !field.IsZero() {
			continue
		}

		if err := setFieldFromString(field, defTag); err != nil {
			return fmt.Errorf("field %s default %q: %w", ft.Name, defTag, err)
		}
	}

	return nil
}

// applyData populates target fields from a map using JSON tag matching.
func applyData(target any, data map[string]any) error {
	if target == nil || len(data) == 0 {
		return nil
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return errors.New("target must point to a struct")
	}

	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		ft := t.Field(i)

		// Skip fields without json tag or with "-".
		jsonTag := ft.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name := jsonFieldName(jsonTag, ft.Name)

		raw, ok := data[name]
		if !ok {
			continue
		}

		if field.Kind() == reflect.Struct {
			subMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}

			if err := applyData(field.Addr().Interface(), subMap); err != nil {
				return fmt.Errorf("field %s: %w", name, err)
			}

			continue
		}

		if err := setFieldFromAny(field, raw); err != nil {
			return fmt.Errorf("field %s: %w", name, err)
		}
	}

	return nil
}

// applyEnv overrides fields from environment variables using `env` struct tags.
// Env vars are looked up as PREFIX_FIELD (e.g. WEAVE_BASH_TIMEOUT).
func applyEnv(target any, prefix string) error {
	if target == nil || prefix == "" {
		return nil
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		ft := t.Field(i)

		// Recurse into nested structs.
		if field.Kind() == reflect.Struct {
			if err := applyEnv(field.Addr().Interface(), prefix); err != nil {
				return err
			}

			continue
		}

		envTag := ft.Tag.Get("env")
		if envTag == "" {
			continue
		}

		key := prefix + "_" + envTag

		val, ok := os.LookupEnv(key)
		if !ok {
			continue
		}

		if err := setFieldFromString(field, val); err != nil {
			return fmt.Errorf("field %s env %s=%q: %w", ft.Name, key, val, err)
		}
	}

	return nil
}

// applyFlags overrides fields from CLI flags using `flag` and `short` struct tags.
// Returns the remaining non-flag args.
func applyFlags(target any, args []string) ([]string, error) {
	if target == nil || len(args) == 0 {
		return args, nil
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, errors.New("target must be a non-nil pointer")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil, errors.New("target must point to a struct")
	}

	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	t := v.Type()
	knownFlags := make(map[string]bool)

	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		ft := t.Field(i)

		// Skip nested structs for flat flag parsing.
		if field.Kind() == reflect.Struct {
			continue
		}

		flagTag := ft.Tag.Get("flag")

		shortTag := ft.Tag.Get("short")
		if flagTag == "" && shortTag == "" {
			continue
		}

		ptr := field.Addr().Interface()

		if flagTag != "" {
			knownFlags["--"+flagTag] = true
			defineFlag(fs, flagTag, ptr)
		}

		if shortTag != "" {
			knownFlags["-"+shortTag] = true
			defineFlag(fs, shortTag, ptr)
		}
	}

	if len(knownFlags) == 0 {
		return args, nil
	}

	// Filter args to only include known flags, tracking consumed indices.
	filtered, consumed := filterKnownFlags(args, knownFlags)
	if len(filtered) == 0 {
		return args, nil
	}

	if err := fs.Parse(filtered); err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	// Return original args minus consumed flags and their values.
	var remaining []string

	for i, arg := range args {
		if !consumed[i] {
			remaining = append(remaining, arg)
		}
	}

	return remaining, nil
}

// filterKnownFlags returns args that match known flags (or their values) and a
// map of consumed indices.
func filterKnownFlags(args []string, known map[string]bool) ([]string, map[int]bool) {
	var result []string

	consumed := make(map[int]bool)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		// Check for --flag=value or -f=value form.
		if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
			if known[arg[:eqIdx]] {
				result = append(result, arg)
				consumed[i] = true
			}

			continue
		}

		if known[arg] {
			result = append(result, arg)
			consumed[i] = true
			// Include the next arg as the value if it doesn't start with -.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result = append(result, args[i+1])
				consumed[i+1] = true
				i++
			}

			continue
		}
	}

	return result, consumed
}

// validate runs all validation rules on the target struct.
func validate(target any) error {
	if target == nil {
		return nil
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}

	var errs ValidationErrors
	if err := validateStruct(v, "", &errs); err != nil {
		return err
	}

	// Check for custom Validate() error interface.
	if validator, ok := target.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			errs = append(errs, ValidationError{
				Field:   "",
				Message: err.Error(),
			})
		}
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

func validateStruct(v reflect.Value, prefix string, errs *ValidationErrors) error {
	t := v.Type()

	for i := range v.NumField() {
		field := v.Field(i)
		ft := t.Field(i)

		fieldName := ft.Name
		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		if field.Kind() == reflect.Struct {
			if err := validateStruct(field, fieldName, errs); err != nil {
				return err
			}

			continue
		}

		validateTag := ft.Tag.Get("validate")
		if validateTag == "" {
			continue
		}

		rules := strings.SplitSeq(validateTag, ",")
		for rule := range rules {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}

			if msg := checkRule(field, rule); msg != "" {
				*errs = append(*errs, ValidationError{
					Field:   fieldName,
					Message: msg,
				})
			}
		}
	}

	return nil
}

const ruleRequired = "required"

func checkRule(field reflect.Value, rule string) string {
	switch {
	case rule == ruleRequired:
		if field.IsZero() {
			return "required field is empty"
		}

		return ""
	case rule == "url":
		if field.Kind() != reflect.String {
			return "url validation only applies to strings"
		}

		val := field.String()
		if val == "" {
			return "" // empty is ok unless required
		}

		if _, err := url.ParseRequestURI(val); err != nil {
			return fmt.Sprintf("invalid URL: %v", err)
		}

		return ""
	case strings.HasPrefix(rule, "gt="):
		return checkNumericCompare(field, rule[3:], "gt")
	case strings.HasPrefix(rule, "lt="):
		return checkNumericCompare(field, rule[3:], "lt")
	case strings.HasPrefix(rule, "min="):
		return checkMin(field, rule[4:])
	case strings.HasPrefix(rule, "max="):
		return checkMax(field, rule[4:])
	case strings.HasPrefix(rule, "oneof="):
		return checkOneOf(field, rule[6:])
	default:
		return "unknown validation rule: " + rule
	}
}

func checkNumericCompare(field reflect.Value, valStr, op string) string {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			return fmt.Sprintf("invalid %s value: %s", op, valStr)
		}

		v := field.Int()
		if op == "gt" && v <= n {
			return fmt.Sprintf("must be greater than %d", n)
		}

		if op == "lt" && v >= n {
			return fmt.Sprintf("must be less than %d", n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			return fmt.Sprintf("invalid %s value: %s", op, valStr)
		}

		v := field.Uint()
		if op == "gt" && v <= n {
			return fmt.Sprintf("must be greater than %d", n)
		}

		if op == "lt" && v >= n {
			return fmt.Sprintf("must be less than %d", n)
		}
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return fmt.Sprintf("invalid %s value: %s", op, valStr)
		}

		v := field.Float()
		if op == "gt" && v <= n {
			return fmt.Sprintf("must be greater than %f", n)
		}

		if op == "lt" && v >= n {
			return fmt.Sprintf("must be less than %f", n)
		}
	default:
		return op + " validation only applies to numeric types"
	}

	return ""
}

func checkMin(field reflect.Value, valStr string) string {
	return checkBound(field, valStr, "min")
}

func checkMax(field reflect.Value, valStr string) string {
	return checkBound(field, valStr, "max")
}

//nolint:gocyclo // validation boundary checks are inherently branch-heavy
func checkBound(field reflect.Value, valStr, bound string) string {
	lessThan := bound == "min"

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			return "invalid " + bound + " value: " + valStr
		}

		v := field.Int()
		if lessThan && v < n {
			return fmt.Sprintf("must be at least %d", n)
		}

		if !lessThan && v > n {
			return fmt.Sprintf("must be at most %d", n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			return "invalid " + bound + " value: " + valStr
		}

		v := field.Uint()
		if lessThan && v < n {
			return fmt.Sprintf("must be at least %d", n)
		}

		if !lessThan && v > n {
			return fmt.Sprintf("must be at most %d", n)
		}
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return "invalid " + bound + " value: " + valStr
		}

		v := field.Float()
		if lessThan && v < n {
			return fmt.Sprintf("must be at least %f", n)
		}

		if !lessThan && v > n {
			return fmt.Sprintf("must be at most %f", n)
		}
	case reflect.String:
		n, err := strconv.Atoi(valStr)
		if err != nil {
			return "invalid " + bound + " value for string: " + valStr
		}

		if lessThan && len(field.String()) < n {
			return fmt.Sprintf("must be at least %d characters", n)
		}

		if !lessThan && len(field.String()) > n {
			return fmt.Sprintf("must be at most %d characters", n)
		}
	case reflect.Slice, reflect.Array:
		n, err := strconv.Atoi(valStr)
		if err != nil {
			return "invalid " + bound + " value for slice: " + valStr
		}

		if lessThan && field.Len() < n {
			return fmt.Sprintf("must have at least %d elements", n)
		}

		if !lessThan && field.Len() > n {
			return fmt.Sprintf("must have at most %d elements", n)
		}
	default:
		return bound + " validation only applies to numeric, string, and slice types"
	}

	return ""
}

func checkOneOf(field reflect.Value, valStr string) string {
	if field.Kind() != reflect.String {
		return "oneof validation only applies to strings"
	}

	val := field.String()
	if val == "" {
		return "" // empty is ok unless required
	}

	options := strings.Fields(valStr)
	if slices.Contains(options, val) {
		return ""
	}

	return "must be one of: " + strings.Join(options, ", ")
}

// setFieldFromString sets a field value from a string representation.
func setFieldFromString(field reflect.Value, s string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("parse int: %w", err)
		}

		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return fmt.Errorf("parse uint: %w", err)
		}

		field.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("parse float: %w", err)
		}

		field.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("parse bool: %w", err)
		}

		field.SetBool(b)
	default:
		// Unsupported kind for default tag; skip.
	}

	return nil
}

// setFieldFromAny sets a field from an arbitrary value, handling type coercion.
//
//nolint:gocyclo // type coercion is inherently branch-heavy by design
func setFieldFromAny(field reflect.Value, raw any) error {
	if raw == nil {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		switch v := raw.(type) {
		case string:
			field.SetString(v)
		case float64:
			field.SetString(strconv.FormatFloat(v, 'f', -1, 64))
		case int:
			field.SetString(strconv.Itoa(v))
		case bool:
			field.SetString(strconv.FormatBool(v))
		default:
			return fmt.Errorf("cannot convert %T to string", raw)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := raw.(type) {
		case float64:
			field.SetInt(int64(v))
		case int:
			field.SetInt(int64(v))
		case int64:
			field.SetInt(v)
		case string:
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return fmt.Errorf("cannot parse %q as int: %w", v, err)
			}

			field.SetInt(n)
		default:
			return fmt.Errorf("cannot convert %T to int", raw)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch v := raw.(type) {
		case float64:
			field.SetUint(uint64(v))
		case int:
			if v < 0 {
				return fmt.Errorf("cannot convert negative int %d to uint", v)
			}

			field.SetUint(uint64(v))
		case uint64:
			field.SetUint(v)
		case string:
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return fmt.Errorf("cannot parse %q as uint: %w", v, err)
			}

			field.SetUint(n)
		default:
			return fmt.Errorf("cannot convert %T to uint", raw)
		}
	case reflect.Float32, reflect.Float64:
		switch v := raw.(type) {
		case float64:
			field.SetFloat(v)
		case float32:
			field.SetFloat(float64(v))
		case int:
			field.SetFloat(float64(v))
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("cannot parse %q as float: %w", v, err)
			}

			field.SetFloat(f)
		default:
			return fmt.Errorf("cannot convert %T to float", raw)
		}
	case reflect.Bool:
		switch v := raw.(type) {
		case bool:
			field.SetBool(v)
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("cannot parse %q as bool: %w", v, err)
			}

			field.SetBool(b)
		default:
			return fmt.Errorf("cannot convert %T to bool", raw)
		}
	case reflect.Slice:
		return setSliceFromAny(field, raw)
	default:
		return fmt.Errorf("unsupported field kind: %s", field.Kind())
	}

	return nil
}

func setSliceFromAny(field reflect.Value, raw any) error {
	switch v := raw.(type) {
	case []any:
		slice := reflect.MakeSlice(field.Type(), len(v), len(v))
		for i, item := range v {
			elem := slice.Index(i)
			if err := setFieldFromAny(elem, item); err != nil {
				return fmt.Errorf("slice element %d: %w", i, err)
			}
		}

		field.Set(slice)
	case []string:
		if field.Type().Elem().Kind() == reflect.String {
			field.Set(reflect.ValueOf(v))
			return nil
		}

		slice := reflect.MakeSlice(field.Type(), len(v), len(v))
		for i, item := range v {
			elem := slice.Index(i)
			if err := setFieldFromAny(elem, item); err != nil {
				return fmt.Errorf("slice element %d: %w", i, err)
			}
		}

		field.Set(slice)
	default:
		return fmt.Errorf("cannot convert %T to slice", raw)
	}

	return nil
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

// defineFlag defines a flag in a FlagSet for the given pointer value.
func defineFlag(fs *flag.FlagSet, name string, ptr any) {
	switch p := ptr.(type) {
	case *string:
		fs.StringVar(p, name, *p, "")
	case *int:
		fs.IntVar(p, name, *p, "")
	case *int64:
		fs.Int64Var(p, name, *p, "")
	case *uint:
		fs.UintVar(p, name, *p, "")
	case *uint64:
		fs.Uint64Var(p, name, *p, "")
	case *float64:
		fs.Float64Var(p, name, *p, "")
	case *bool:
		fs.BoolVar(p, name, *p, "")
	}
}
