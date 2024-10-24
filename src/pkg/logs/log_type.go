package logs

import (
	"fmt"
	"strings"
)

type LogType uint32

type InvalidLogTypeError struct {
	Value string
}

func (e InvalidLogTypeError) Error() string {
	return fmt.Sprintf("invalid log type: %q, must be one of %v", e.Value, AllLogTypes)
}

const (
	LogTypeUnspecified LogType = 0
	LogTypeRun         LogType = 1 << iota
	LogTypeBuild

	LogTypeAll LogType = 0xFFFFFFFF
)

var AllLogTypes = []LogType{
	LogTypeRun,
	LogTypeBuild,
}

var (
	LogType_name = map[LogType]string{
		LogTypeUnspecified: "UNSPECIFIED",
		LogTypeRun:         "RUN",
		LogTypeBuild:       "BUILD",
		LogTypeAll:         "ALL",
	}
	LogType_value = map[string]LogType{
		"UNSPECIFIED": LogTypeUnspecified,
		"RUN":         LogTypeRun,
		"BUILD":       LogTypeBuild,
		"ALL":         LogTypeAll,
	}
)

func (c *LogType) Set(value string) error {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "ALL" {
		*c = LogTypeAll
		return nil
	}

	for _, logType := range AllLogTypes {
		if logType.String() == value {
			*c |= logType
			return nil
		}
	}

	return InvalidLogTypeError{Value: value}
}

func (c LogType) Has(logType LogType) bool {
	return c&logType != 0
}

func (c LogType) Value() string {
	return c.String()
}

func ParseLogType(value string) (LogType, error) {
	var logType LogType
	if value == "" {
		return logType, nil
	}

	parts := strings.Split(value, ",")
	for _, part := range parts {
		if err := logType.Set(part); err != nil {
			return 0, err
		}
		logType |= logType
	}

	return logType, nil
}

func (c LogType) String() string {
	// convert the bitfield into a comma-separated list of log types
	var logTypes []string
	for _, logType := range AllLogTypes {
		if c&logType != 0 {
			logTypes = append(logTypes, LogType_name[logType])
		}
	}

	if len(logTypes) == 0 {
		return LogType_name[LogTypeUnspecified]
	}

	return strings.Join(logTypes, ",")
}
