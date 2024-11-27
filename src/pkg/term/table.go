package term

import (
	"errors"
	"fmt"
	"reflect"
	"text/tabwriter"
)

func Table(slice interface{}, attributes []string) error {
	// Ensure slice is a slice
	val := reflect.ValueOf(slice)
	if val.Kind() != reflect.Slice {
		return errors.New("Table: input is not a slice")
	}

	// Create a tabwriter
	w := tabwriter.NewWriter(DefaultTerm.outw, 0, 0, 2, ' ', 0)

	var err error

	// Print headers
	for _, attr := range attributes {
		_, err = fmt.Fprintf(w, "%s\t", attr)
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(w)
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
				_, err = fmt.Fprintf(w, "N/A\t")
				if err != nil {
					return err
				}
				continue
			}
			_, err = fmt.Fprintf(w, "%v\t", field.Interface())
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
