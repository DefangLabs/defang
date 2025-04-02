package compose

import "testing"

func TestDetectConfig(t *testing.T) {
	tests := []struct {
		input          string
		expectedOutput []string
	}{
		{"", nil},
		{"not a secret", nil},
		{"/leaderboard/api/hubs", nil},
		{"https://user:p455w0rd@example.com", []string{"URL with password"}},
		{"LINK: https://user:p455w0rd@example.com, LINK: https://user:p845w0rd@example.com", []string{"URL with password", "URL with password"}},
		{"api-key=50m34p1k3y", []string{"Keyword Detector"}},
		{"VEfk5vO0Q53VkK_uicor", []string{"High entropy string"}},
		{"ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ1234567890", []string{"Github authentication"}},
		{"AROA1234567890ABCDEF", []string{"AWS Client ID"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ds, err := detectConfig(tt.input)

			//check for error
			if err != nil {
				if len(tt.expectedOutput) > 0 && tt.expectedOutput[0] != "" {
					t.Errorf("Error: %v", err)
				}
				return
			}

			// check for length of the output
			if len(ds) != len(tt.expectedOutput) {
				t.Errorf("Expected %d detector types, but got %d", len(tt.expectedOutput), len(ds))
				return
			}

			// check for the output values
			for i, d := range ds {
				if d != tt.expectedOutput[i] {
					t.Errorf("Expected detector type %s, but got %s", tt.expectedOutput[i], d)
				}
			}
		})
	}
}
