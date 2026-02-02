package term

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/tabwriter"
)

func Table(slice any, attributes ...string) error {
	return DefaultTerm.Table(slice, attributes...)
}

func (t *Term) Table(slice any, attributes ...string) error {
	if t.json {
		return t.jsonTable(slice, attributes...)
	}
	return t.table(slice, attributes...)
}

func (t *Term) jsonTable(slice any, attributes ...string) error {
	val := reflect.ValueOf(slice)
	if val.Kind() != reflect.Slice {
		return errors.New("Table: input is not a slice")
	}

	encoder := json.NewEncoder(t.out)
	encoder.SetIndent("", "\t")

	filtered := make([]map[string]any, val.Len())
	for i := range val.Len() {
		item := val.Index(i)
		if item.Kind() == reflect.Ptr {
			item = item.Elem()
		}

		filtered[i] = make(map[string]any)
		for _, attr := range attributes {
			field := item.FieldByName(attr)
			if !field.IsValid() {
				continue
			}

			// Use json tag if available, otherwise use field name
			jsonKey := attr
			if structField, ok := item.Type().FieldByName(attr); ok {
				if tag := structField.Tag.Get("json"); tag != "" && tag != "-" {
					jsonKey = strings.Split(tag, ",")[0]
				}
			}
			filtered[i][jsonKey] = field.Interface()
		}
	}

	return encoder.Encode(filtered)
}

func (t *Term) table(slice any, attributes ...string) error {
	// Ensure slice is a slice
	val := reflect.ValueOf(slice)
	if val.Kind() != reflect.Slice {
		return errors.New("Table: input is not a slice")
	}

	// Create a tabwriter
	w := tabwriter.NewWriter(t.out, 0, 0, 2, ' ', 0)

	var err error

	var resetBold string
	if t.StdoutCanColor() {
		fmt.Fprintln(w, boldColorStr) // must be separate line or it will be counted as part of the 1st header
		resetBold = resetColorStr
	}

	// Print headers
	for i, attr := range attributes {
		var prefix string
		if i > 0 {
			prefix = "\t"
		}
		_, err = fmt.Fprint(w, prefix, strings.ToUpper(attr))
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(w, resetBold)
	if err != nil {
		return err
	}

	// Print rows
	for i := range val.Len() {
		item := val.Index(i)
		if item.Kind() == reflect.Ptr {
			item = item.Elem()
		}

		for _, attr := range attributes {
			field := item.FieldByName(attr)
			if !field.IsValid() {
				_, err = fmt.Fprint(w, "N/A\t")
				if err != nil {
					return err
				}
				continue
			}
			val := field.Interface()
			zero := reflect.Zero(field.Type()).Interface()
			if reflect.DeepEqual(val, zero) {
				_, err = fmt.Fprint(w, "\t")
			} else {
				_, err = fmt.Fprintf(w, "%v\t", val)
			}
			if err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(w)
		if err != nil {
			return err
		}
	}

	return w.Flush()
}
