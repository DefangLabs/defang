package logs

import "testing"

func TestParseLogType(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    LogType
		wantErr bool
	}{
		{"empty", "", LogTypeUnspecified, false},
		{"unspecified", "unspecified", LogTypeUnspecified, true},
		{"run", "run", LogTypeRun, false},
		{"build", "build", LogTypeBuild, false},
		{"all", "all", LogTypeAll, false},
		{"invalid", "invalid", LogTypeUnspecified, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLogType(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLogType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseLogType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogTypeString(t *testing.T) {
	tests := []struct {
		name  string
		value LogType
		want  string
	}{
		{"unspecified", LogTypeUnspecified, "UNSPECIFIED"},
		{"run", LogTypeRun, "RUN"},
		{"build", LogTypeBuild, "BUILD"},
		{"all", LogTypeAll, "RUN,BUILD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.String(); got != tt.want {
				t.Errorf("LogType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogTypeSet(t *testing.T) {
	tests := []struct {
		name    string
		init    string
		value   string
		want    LogType
		wantErr bool
	}{
		{"empty from unspecified", "", "", LogTypeUnspecified, true},
		{"run from unspecified", "", "run", LogTypeRun, false},
		{"build from unspecified", "", "build", LogTypeBuild, false},
		{"all from unspecified", "", "all", LogTypeAll, false},
		{"invalid from unspecified", "", "invalid", LogTypeUnspecified, true},
		{"empty from run", "", "", LogTypeRun, true},
		{"run from run", "", "run", LogTypeRun, false},
		{"build from run", "", "build", LogTypeAll, false},
		{"all from run", "", "all", LogTypeAll, false},
		{"invalid from run", "", "invalid", LogTypeRun, true},
		{"empty from build", "", "", LogTypeBuild, true},
		{"run from build", "", "run", LogTypeAll, false},
		{"build from build", "", "build", LogTypeBuild, false},
		{"all from build", "", "all", LogTypeAll, false},
		{"invalid from build", "", "invalid", LogTypeRun, true},
		{"empty from all", "", "", LogTypeAll, true},
		{"run from all", "", "run", LogTypeAll, false},
		{"build from all", "", "build", LogTypeAll, false},
		{"all from all", "", "all", LogTypeAll, false},
		{"invalid from all", "", "invalid", LogTypeAll, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logType, err := ParseLogType(tt.init)
			if err != nil {
				t.Errorf("ParseLogType() error = %v", err)
				return
			}
			if err := logType.Set(tt.value); (err != nil) != tt.wantErr {
				t.Errorf("LogType.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
