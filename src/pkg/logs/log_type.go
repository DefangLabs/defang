package logs

import (
	"fmt"
	"strings"
)

type LogType uint8

type InvalidLogTypeError struct {
	Value string
}

func (e InvalidLogTypeError) Error() string {
	return fmt.Sprintf("invalid log type: %q, must be one of %v", e.Value, AllLogTypes)
}

const (
	LogTypeUnspecified LogType = 0
	LogTypeRun         LogType = 1
	LogTypeBuild       LogType = 2
	// this value is used as a bitfield
	// LogTypeNext1 LogType = 4
	// LogTypeNext2 LogType = 8
	LogTypeAll LogType = 255
)

var AllLogTypes = []LogType{
	LogTypeRun,
	LogTypeBuild,
}

var (
	LogType_name = map[LogType]string{
		0: "UNSPECIFIED",
		1: "RUN",
		2: "BUILD",
		// this value is used as a bitfield
		// 4: "NEXT1",
		// 8: "NEXT2",
		255: "ALL",
	}
	LogType_value = map[string]LogType{
		"UNSPECIFIED": 0,
		"RUN":         1,
		"BUILD":       2,
		// this value is used as a bitfield
		// "NEXT1": 4,
		// "NEXT2": 8,
		"ALL": 255,
	}
)

func (c *LogType) Set(value string) error {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "ALL" {
		*c = LogType(255)
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
		return "UNSPECIFIED"
	}

	return strings.Join(logTypes, ",")
}
