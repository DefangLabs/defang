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
		{"REDIS_URL=rediss://foo:p41fce90d44ac1d891bd21fdbc5dfc1bd7f163e33a6934c30093eaf56c1c23937@ec2-98-85-106-43.compute-1.amazonaws.com:8240", []string{"URL with password"}},
		{"REDIS_URL=rediss://:p41fce90d44ac1d891bd21fdbc5dfc1bd7f163e33a6934c30093eaf56c1c23937@ec2-98-85-106-43.compute-1.amazonaws.com:8240", []string{"URL with password"}},
		{"ENCRYPTION_KEY: 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", []string{"High entropy string"}},
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
					t.Errorf("Expected detector type %s, but got: %s", tt.expectedOutput[i], d)
				}
			}
		})
	}
}
