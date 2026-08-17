package timeutils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ParseDuration parses a Go duration with an optional whole-days prefix,
// like "12h", "7d" or "7d12h". A negative remainder after a days prefix
// (like "7d-1h") is rejected; without the prefix, Go's sign rules apply.
func ParseDuration(str string) (time.Duration, error) {
	if i := strings.IndexByte(str, 'd'); i > 0 {
		days, err := strconv.ParseUint(str[:i], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", str)
		}
		dur := time.Duration(days) * 24 * time.Hour
		rest := str[i+1:]
		if rest == "" {
			return dur, nil
		}
		rd, err := time.ParseDuration(rest)
		if err != nil || rd < 0 {
			return 0, fmt.Errorf("invalid duration %q", str)
		}
		return dur + rd, nil
	}
	return time.ParseDuration(str)
}

// ParseTimeOrDuration parses a time string or duration string (e.g. 1h30m or 7d) and returns a time.Time.
// At a minimum, this function supports RFC3339Nano, Go durations with an optional whole-days prefix, and our own TimestampFormat (local).
func ParseTimeOrDuration(str string, now time.Time) (time.Time, error) {
	if str == "" {
		return time.Time{}, nil
	}
	if strings.ContainsAny(str, "TZ") {
		return time.Parse(time.RFC3339Nano, str)
	}
	if strings.Contains(str, ":") {
		local, err := time.ParseInLocation("15:04:05.999999", str, time.Local)
		if err != nil {
			return time.Time{}, err
		}
		// Replace the year, month, and day of t with today's date
		now := now.Local()
		sincet := time.Date(now.Year(), now.Month(), now.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), local.Location())
		if sincet.After(now) {
			sincet = sincet.AddDate(0, 0, -1) // yesterday; subtract 1 day
		}
		return sincet, nil
	}
	dur, err := ParseDuration(str)
	if err != nil {
		// try as unix millis or seconds
		if unix, parseErr := strconv.ParseFloat(str, 64); parseErr == nil && unix < 1e13 {
			// Float64 has 53 bits of precision, which means up to ~1e16 is lossless,
			// but unix timestamps in nanoseconds is ~1e19 which exceeds that.
			// This is why we stick to milliseconds precision in the float64,
			// but convert to nanoseconds after conversion to int64.
			if unix < 1e10 {
				unix *= 1e3 // convert seconds to milliseconds
			}
			return time.Unix(0, int64(unix)*1e6), nil
		}
		return time.Time{}, err
	}
	return now.Add(-dur), nil // - because we want to go back in time
}

func AsTime(ts *timestamppb.Timestamp, def time.Time) time.Time {
	if !ts.IsValid() { // handles nil too
		return def
	}
	return ts.AsTime()
}
