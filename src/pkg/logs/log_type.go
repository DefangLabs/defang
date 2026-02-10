package logs

import (
	"fmt"
	"strings"
)

type LogType uint32

type ErrInvalidLogType struct {
	Value string
}

func (e ErrInvalidLogType) Error() string {
	return fmt.Sprintf("invalid log type: %q, must be one of %v", e.Value, AllLogTypes)
}

const (
	LogTypeUnspecified LogType = 0
	LogTypeCD          LogType = 1 << iota
	LogTypeRun
	LogTypeBuild
	LogTypeAll LogType = 0xFFFFFFFF
)

var AllLogTypes = []LogType{
	LogTypeRun,
	LogTypeBuild,
	LogTypeAll,
}

var (
	logType_name = map[LogType]string{
		LogTypeUnspecified: "UNSPECIFIED",
		LogTypeCD:          "CD",
		LogTypeRun:         "RUN",
		LogTypeBuild:       "BUILD",
		LogTypeAll:         "ALL",
	}
	logType_value = map[string]LogType{
		"UNSPECIFIED": LogTypeUnspecified,
		"CD":          LogTypeCD,
		"RUN":         LogTypeRun,
		"BUILD":       LogTypeBuild,
		"ALL":         LogTypeAll,
	}
)

func ParseLogType(value string) (LogType, error) {
	var logType LogType

	if value != "" {
		value = strings.TrimSpace(strings.ToUpper(value))

		parts := strings.Split(value, ",")
		for _, part := range parts {
			bit, ok := logType_value[part]
			if !ok {
				return 0, ErrInvalidLogType{Value: value}
			}

			logType |= bit
		}
	}
	return logType, nil
}

func (c *LogType) Set(value string) error {
	var err error
	*c, err = ParseLogType(value)
	return err
}

func (c LogType) Has(logType LogType) bool {
	return logType != LogTypeUnspecified && (c&logType) == logType
}

func (c LogType) Type() string {
	return "log-type"
}

func (c LogType) String() string {
	if exact := logType_name[c]; exact != "" {
		return exact
	}
	// convert the bitfield into a comma-separated list of log types
	var logTypes []string
	for _, logType := range AllLogTypes {
		if c.Has(logType) {
			logTypes = append(logTypes, logType_name[logType])
		}
	}

	if len(logTypes) == 0 {
		return logType_name[LogTypeUnspecified]
	}

	return strings.Join(logTypes, ",")
}
