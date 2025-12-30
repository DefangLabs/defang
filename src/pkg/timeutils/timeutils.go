package timeutils

import (
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ParseTimeOrDuration parses a time string or duration string (e.g. 1h30m) and returns a time.Time.
// At a minimum, this function supports RFC3339Nano, Go durations, and our own TimestampFormat (local).
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
	dur, err := time.ParseDuration(str)
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
