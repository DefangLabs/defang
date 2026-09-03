package stackpath

import "testing"

func TestStackDir(t *testing.T) {
	tests := []struct {
		name                            string
		prefix, project, stack, groupID string
		want                            string
	}{
		{"with prefix", "Defang", "myproject", "beta", LogGroupECS, "/Defang/myproject/beta/ecs"},
		{"empty prefix drops its segment", "", "myproject", "beta", LogGroupECS, "/myproject/beta/ecs"},
		{"builds", "Defang", "myproject", "beta", LogGroupBuilds, "/Defang/myproject/beta/builds"},
		{"services", "Defang", "myproject", "beta", LogGroupServices, "/Defang/myproject/beta/logs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StackDir(tt.prefix, tt.project, tt.stack, tt.groupID); got != tt.want {
				t.Errorf("StackDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsLogGroup(t *testing.T) {
	tests := []struct {
		name, identifier, group string
		want                    bool
	}{
		{"bare name", "/Defang/myproject/beta/ecs", LogGroupECS, true},
		{"account-qualified live tail", "532501343364:/Defang/django/beta/ecs", LogGroupECS, true},
		{"arn", "arn:aws:logs:us-west-2:123:log-group:/Defang/myproject/beta/ecs", LogGroupECS, true},
		{"wrong group", "/Defang/myproject/beta/builds", LogGroupECS, false},
		{"suffix must follow a slash", "/Defang/myproject/beta/notecs", LogGroupECS, false},
		// A lone segment is not a log group identifier: every shape CloudWatch
		// produces carries the full StackDir path. Matching it would weaken the
		// slash guard above for no reachable caller.
		{"lone segment is not an identifier", "ecs", LogGroupECS, false},
		{"builds", "/Defang/myproject/beta/builds", LogGroupBuilds, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLogGroup(tt.identifier, tt.group); got != tt.want {
				t.Errorf("IsLogGroup(%q, %q) = %v, want %v", tt.identifier, tt.group, got, tt.want)
			}
		})
	}
}
