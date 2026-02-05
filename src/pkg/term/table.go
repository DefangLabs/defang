package term

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"text/tabwriter"
)

func Table(data any, attributes ...string) error {
	return DefaultTerm.Table(data, attributes...)
}

func (t *Term) Table(data any, attributes ...string) error {
	if t.json {
		return t.jsonTable(data)
	}
	return t.table(data, attributes...)
}

func (t *Term) jsonTable(data any) error {
	encoder := json.NewEncoder(t.out)
	encoder.SetIndent("", "\t")
	return encoder.Encode(data)
}

func (t *Term) table(data any, attributes ...string) error {
	// Ensure data is a slice
	val := reflect.ValueOf(data)
	if val.Kind() != reflect.Slice {
		val = reflect.ValueOf([]any{data})
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
		if item.Kind() == reflect.Ptr || item.Kind() == reflect.Interface {
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
