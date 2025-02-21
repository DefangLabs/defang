package compose

import "testing"

func TestDetectConfig(t *testing.T) {
	type Expected struct {
		DetectorType string
		Key          string
		Value        string
	}
	tests := []struct {
		input    string
		expected Expected
	}{
		{"basic dTpw", Expected{DetectorType: "HTTP Basic Authentication", Key: "", Value: "basic dTpw"}},
		{"https://user:p455w0rd@example.com", Expected{DetectorType: "URL with password", Key: "", Value: "https://user:p455w0rd@example.com"}},
	}

	for _, tt := range tests {
		ds, err := detectConfig(tt.input)
		if err != nil {
			t.Errorf("Error: %v", err)
		}
		for _, d := range ds {
			if d.Type != tt.expected.DetectorType {
				t.Errorf("Expected detector type %s, but got %s", tt.expected.DetectorType, d.Type)
			}
			if d.Key != tt.expected.Key {
				t.Errorf("Expected key %s, but got %s", tt.expected.Key, d.Key)
			}
			if d.Value != tt.expected.Value {
				t.Errorf("Expected value %s, but got %s", tt.expected.Value, d.Value)
			}
		}
	}

}
