package contract

import "testing"

func TestConfigNames(t *testing.T) {
	for _, tt := range []struct{ prefix, stack, want string }{
		{"Defang", "prod", "Defang_demo_prod_KEY"},
		{"Custom", "team/prod", "Custom_demo_team_prod_KEY"},
		{"", "prod", "demo_prod_KEY"},
	} {
		if got := ConfigName(tt.prefix, "demo", tt.stack, "KEY"); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
		if got := ConfigPrefix(tt.prefix, "demo", tt.stack); got+"KEY" != tt.want {
			t.Errorf("wrong prefix: %q", got)
		}
	}
}
