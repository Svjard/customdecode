// Package customdecode is a package for populating structs from custom derived
// variables, using struct tags.
package customdecode

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidTarget indicates that the target value passed to
// Decode is invalid.  Target must be a non-nil pointer to a struct.
var ErrInvalidTarget = errors.New("target must be non-nil pointer to struct that has at least one exported field with a valid env tag.")
var ErrNoTargetFieldsAreSet = errors.New("none of the target fields were set from environment variables")

// FailureFunc is called when an error is encountered during a MustDecode
// operation. It prints the error and terminates the process.
//
// This variable can be assigned to another function of the user-programmer's
// design, allowing for graceful recovery of the problem, such as loading
// from a backup configuration file.
var FailureFunc = func(err error) {
    log.Fatalf("envdecode: an error was encountered while decoding: %v\n", err)
}

// Decoder is the interface implemented by an object that can decode an
// environment variable string representation of itself.
type Decoder interface {
	Decode(string) error
}

// VarFetch is a function which takes the name of a variable and fetches the
// value in string representation
type VarFetch func(string) string

// Decode custom derived variables into the provided target.  The target
// must be a non-nil pointer to a struct.  Fields in the struct must
// be exported, and tagged with an "custom" struct tag with a value
// containing the name of the custom derived variable.  An error is
// returned if there are no exported members tagged.
//
// Default values may be provided by appending ",default=value" to the
// struct tag.  Required values may be marked by appending ",required"
// to the struct tag.  It is an error to provide both "default" and
// "required". Strict values may be marked by appending ",strict" which
// will return an error on Decode if there is an error while parsing.
// If everything must be strict, consider using StrictDecode instead.
//
// All primitive types are supported, including bool, floating point,
// signed and unsigned integers, and string.  Boolean and numeric
// types are decoded using the standard strconv Parse functions for
// those types.  Structs and pointers to structs are decoded
// recursively.  time.Duration is supported via the
// time.ParseDuration() function and *url.URL is supported via the
// url.Parse() function. Slices are supported for all above mentioned
// primitive types. Semicolon is used as delimiter in custom derived variables.
func Decode(target interface{}, fn VarFetch, options ...Option) error {
	cfg := newConfig(options...)
	nFields, err := decode(target, fn, cfg)
	if err != nil {
		return err
	}

	// if we didn't do anything - the user probably did something
	// wrong like leave all fields unexported.
	if nFields == 0 {
		return ErrNoTargetFieldsAreSet
	}

	return nil
}

// StrictDecode is similar to Decode except all fields will have an implicit
// ",strict" on all fields.
func StrictDecode(target interface{}, fn VarFetch, options ...Option) error {
	options = append(options, WithStrictDecoding())
	cfg := newConfig(options...)
	nFields, err := decode(target, fn, cfg)
	if err != nil {
		return err
	}

	// if we didn't do anything - the user probably did something
	// wrong like leave all fields unexported.
	if nFields == 0 {
		return ErrInvalidTarget
	}

	return nil
}

func decode(target interface{}, fn VarFetch, cfg config) (int, error) {
	s := reflect.ValueOf(target)
	if s.Kind() != reflect.Ptr || s.IsNil() {
		return 0, ErrInvalidTarget
	}

	s = s.Elem()
	if s.Kind() != reflect.Struct {
		return 0, ErrInvalidTarget
	}

	t := s.Type()
	setFieldCount := 0
	for i := 0; i < s.NumField(); i++ {
		// Localize the umbrella `strict` value to the specific field.
		strict := cfg.strict

		f := s.Field(i)

		switch f.Kind() {
		case reflect.Ptr:
			if f.Elem().Kind() != reflect.Struct {
				break
			}

			f = f.Elem()
			fallthrough

		case reflect.Struct:
			if !f.Addr().CanInterface() {
				continue
			}

			ss := f.Addr().Interface()
			_, custom := ss.(Decoder)
			if custom {
				break
			}

			n, err := decode(ss, fn, cfg)
			if err != nil {
				return 0, err
			}
			setFieldCount += n
		}

		if !f.CanSet() {
			continue
		}

		tag := t.Field(i).Tag.Get("custom")
		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")
		val := fn(parts[0])

		required := cfg.require
		hasDefault := false
		defaultValue := ""

		for _, o := range parts[1:] {
			if !required {
				required = strings.HasPrefix(o, "required")
			}
			if strings.HasPrefix(o, "default=") {
				hasDefault = true
				defaultValue = o[8:]
			}
			if !strict {
				strict = strings.HasPrefix(o, "strict")
			}
		}

		if cfg.nodefaults {
			hasDefault = false
			defaultValue = ""
		}

		if required && hasDefault {
			panic(`customdecode: "default" and "required" may not be specified in the same annotation`)
		}
		if val == "" && required {
			return 0, fmt.Errorf("the variable \"%s\" is missing", parts[0])
		}
		if val == "" {
			val = defaultValue
		}
		if val == "" {
			continue
		}

		setFieldCount++

		decoder, custom := f.Addr().Interface().(Decoder)
		if custom {
			if err := decoder.Decode(val); err != nil {
				return 0, err
			}
		} else if f.Kind() == reflect.Slice {
			decodeSlice(&f, val)
		} else {
			if err := decodePrimitiveType(&f, val); err != nil && strict {
				return 0, err
			}
		}
	}

	return setFieldCount, nil
}

func decodeSlice(f *reflect.Value, val string) {
	parts := strings.Split(val, ";")

	values := parts[:0]
	for _, x := range parts {
		if x != "" {
			values = append(values, strings.TrimSpace(x))
		}
	}

	valuesCount := len(values)
	slice := reflect.MakeSlice(f.Type(), valuesCount, valuesCount)
	if valuesCount > 0 {
		for i := 0; i < valuesCount; i++ {
			e := slice.Index(i)
			decodePrimitiveType(&e, values[i])
		}
	}

	f.Set(slice)
}

func decodePrimitiveType(f *reflect.Value, val string) error {
	switch f.Kind() {
	case reflect.Bool:
		v, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		f.SetBool(v)

	case reflect.Float32, reflect.Float64:
		bits := f.Type().Bits()
		v, err := strconv.ParseFloat(val, bits)
		if err != nil {
			return err
		}
		f.SetFloat(v)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if t := f.Type(); t.PkgPath() == "time" && t.Name() == "Duration" {
			v, err := time.ParseDuration(val)
			if err != nil {
				return err
			}
			f.SetInt(int64(v))
		} else {
			bits := f.Type().Bits()
			v, err := strconv.ParseInt(val, 0, bits)
			if err != nil {
				return err
			}
			f.SetInt(v)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bits := f.Type().Bits()
		v, err := strconv.ParseUint(val, 0, bits)
		if err != nil {
			return err
		}
		f.SetUint(v)

	case reflect.String:
		f.SetString(val)

	case reflect.Ptr:
		if t := f.Type().Elem(); t.Kind() == reflect.Struct && t.PkgPath() == "net/url" && t.Name() == "URL" {
			v, err := url.Parse(val)
			if err != nil {
				return err
			}
			f.Set(reflect.ValueOf(v))
		}
	}
	return nil
}

// MustDecode calls Decode and terminates the process if any errors
// are encountered.
func MustDecode(target interface{}, fn VarFetch) {
	err := Decode(target, fn)
	if err != nil {
		FailureFunc(err)
	}
}

// MustStrictDecode calls StrictDecode and terminates the process if any errors
// are encountered.
func MustStrictDecode(target interface{}, fn VarFetch) {
	err := StrictDecode(target, fn)
	if err != nil {
		FailureFunc(err)
	}
}

//// Configuration info for Export

type ConfigInfo struct {
	Field        string
	CustomVar    string
	Value        string
	DefaultValue string
	HasDefault   bool
	Required     bool
	UsesFn      bool
}

type ConfigInfoSlice []*ConfigInfo

func (c ConfigInfoSlice) Less(i, j int) bool {
	return c[i].CustomVar < c[j].CustomVar
}
func (c ConfigInfoSlice) Len() int {
	return len(c)
}
func (c ConfigInfoSlice) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// Returns a list of final configuration metadata sorted by var name
func Export(target interface{}, fn VarFetch) ([]*ConfigInfo, error) {
	s := reflect.ValueOf(target)
	if s.Kind() != reflect.Ptr || s.IsNil() {
		return nil, ErrInvalidTarget
	}

	cfg := []*ConfigInfo{}

	s = s.Elem()
	if s.Kind() != reflect.Struct {
		return nil, ErrInvalidTarget
	}

	t := s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		fName := t.Field(i).Name

		fElem := f
		if f.Kind() == reflect.Ptr {
			fElem = f.Elem()
		}

		if fElem.Kind() == reflect.Struct {
			ss := fElem.Addr().Interface()
			subCfg, err := Export(ss, fn)
			if err != ErrInvalidTarget {
				f = fElem
				for _, v := range subCfg {
					v.Field = fmt.Sprintf("%s.%s", fName, v.Field)
					cfg = append(cfg, v)
				}
			}
		}

		tag := t.Field(i).Tag.Get("custom")
		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")

		ci := &ConfigInfo{
			Field:   fName,
			CustomVar: parts[0],
			UsesFn: fn(parts[0]) != "",
		}

		for _, o := range parts[1:] {
			if strings.HasPrefix(o, "default=") {
				ci.HasDefault = true
				ci.DefaultValue = o[8:]
			} else if strings.HasPrefix(o, "required") {
				ci.Required = true
			}
		}

		if f.Kind() == reflect.Ptr && f.IsNil() {
			ci.Value = ""
		} else if stringer, ok := f.Interface().(fmt.Stringer); ok {
			ci.Value = stringer.String()
		} else {
			switch f.Kind() {
			case reflect.Bool:
				ci.Value = strconv.FormatBool(f.Bool())

			case reflect.Float32, reflect.Float64:
				bits := f.Type().Bits()
				ci.Value = strconv.FormatFloat(f.Float(), 'f', -1, bits)

			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				ci.Value = strconv.FormatInt(f.Int(), 10)

			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				ci.Value = strconv.FormatUint(f.Uint(), 10)

			case reflect.String:
				ci.Value = f.String()

			case reflect.Slice:
				ci.Value = fmt.Sprintf("%v", f.Interface())

			default:
				// Unable to determine string format for value
				return nil, ErrInvalidTarget
			}
		}

		cfg = append(cfg, ci)
	}

	// No configuration tags found, assume invalid input
	if len(cfg) == 0 {
	    return nil, ErrInvalidTarget
	}

	sort.Sort(ConfigInfoSlice(cfg))

	return cfg, nil
}
