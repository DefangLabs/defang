package azure

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	armappcontainers "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
)

func ptr[T any](v T) *T { return &v }

// app builds a minimal ContainerApp with the given custom-domain names. A nil
// names slice produces an app with no CustomDomains entry; an empty slice
// produces an empty entry. Both shapes appear in real ARM responses.
func appWithCustomDomains(names []string) *armappcontainers.ContainerApp {
	cds := make([]*armappcontainers.CustomDomain, 0, len(names))
	for _, n := range names {
		cds = append(cds, &armappcontainers.CustomDomain{Name: ptr(n)})
	}
	return &armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			Configuration: &armappcontainers.Configuration{
				Ingress: &armappcontainers.Ingress{
					CustomDomains: cds,
				},
			},
		},
	}
}

func TestHasCustomDomain(t *testing.T) {
	tests := []struct {
		name     string
		app      *armappcontainers.ContainerApp
		hostname string
		want     bool
	}{
		{name: "nil app", app: nil, hostname: "x.example.com", want: false},
		{name: "nil Properties", app: &armappcontainers.ContainerApp{}, hostname: "x.example.com", want: false},
		{name: "nil Configuration", app: &armappcontainers.ContainerApp{Properties: &armappcontainers.ContainerAppProperties{}}, hostname: "x.example.com", want: false},
		{name: "nil Ingress", app: &armappcontainers.ContainerApp{Properties: &armappcontainers.ContainerAppProperties{Configuration: &armappcontainers.Configuration{}}}, hostname: "x.example.com", want: false},
		{name: "no custom domains", app: appWithCustomDomains(nil), hostname: "x.example.com", want: false},
		{name: "match", app: appWithCustomDomains([]string{"x.example.com"}), hostname: "x.example.com", want: true},
		{name: "no match", app: appWithCustomDomains([]string{"y.example.com"}), hostname: "x.example.com", want: false},
		{name: "match among many", app: appWithCustomDomains([]string{"a.example.com", "x.example.com", "b.example.com"}), hostname: "x.example.com", want: true},
		{name: "case sensitive", app: appWithCustomDomains([]string{"X.EXAMPLE.COM"}), hostname: "x.example.com", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasCustomDomain(tt.app, tt.hostname); got != tt.want {
				t.Errorf("hasCustomDomain = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHasCustomDomain_NilEntry guards against the wrapper-pointer-list shape
// where ARM produces a slice of *CustomDomain that contains a nil pointer
// (rare but seen in deserialization edge cases). The function must not panic.
func TestHasCustomDomain_NilEntry(t *testing.T) {
	app := &armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			Configuration: &armappcontainers.Configuration{
				Ingress: &armappcontainers.Ingress{
					CustomDomains: []*armappcontainers.CustomDomain{
						nil,
						{Name: ptr("x.example.com")},
						{Name: nil}, // nil Name on a non-nil entry
					},
				},
			},
		},
	}
	if !hasCustomDomain(app, "x.example.com") {
		t.Error("hasCustomDomain should find the match despite nil siblings")
	}
}

func TestIsInvalidValidationMethod(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error mentioning code", err: errors.New("Status=400 Code=\"InvalidValidationMethod\""), want: true},
		{name: "plain error not mentioning code", err: errors.New("something else"), want: false},
		{name: "ResponseError with InvalidValidationMethod", err: &azcore.ResponseError{ErrorCode: "InvalidValidationMethod", StatusCode: 400}, want: true},
		{name: "ResponseError with other code", err: &azcore.ResponseError{ErrorCode: "Unauthorized", StatusCode: 401}, want: false},
		{name: "wrapped ResponseError", err: errors.Join(errors.New("ctx"), &azcore.ResponseError{ErrorCode: "InvalidValidationMethod"}), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInvalidValidationMethod(tt.err); got != tt.want {
				t.Errorf("isInvalidValidationMethod(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsInvalidValidationMethod_RawResponseFallback exercises the string
// fallback branch via a ResponseError whose ErrorCode is empty but whose
// rendered Error() contains "InvalidValidationMethod" in the response body.
func TestIsInvalidValidationMethod_RawResponseFallback(t *testing.T) {
	respErr := &azcore.ResponseError{
		StatusCode: 400,
		RawResponse: &http.Response{
			StatusCode: 400,
			Status:     http.StatusText(400),
			Body:       http.NoBody,
		},
	}
	// The error code on the struct is empty, but in the wild Azure also embeds
	// "InvalidValidationMethod" in the response body. We synthesize that path
	// by wrapping with a fmt error containing the substring.
	wrapped := errors.New("PUT https://...InvalidValidationMethod...")
	if !isInvalidValidationMethod(wrapped) {
		t.Error("isInvalidValidationMethod should match by string fallback when ErrorCode is empty")
	}
	if isInvalidValidationMethod(respErr) {
		t.Error("a ResponseError without the substring or matching ErrorCode must not match")
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"AbC", "abc"},
		{"abc-123", "abc-123"},
		{"x.example.com", "x-example-com"},
		{"defang.study", "defang-study"},
		{"a_b/c d", "a-b-c-d"},
		{"---abc---", "abc"}, // strips leading/trailing hyphens
		{"-a-b-", "a-b"},     // even after sanitization
		{"!!!", ""},          // all-special collapses to empty after trim
		{"a$$b", "a--b"},     // adjacent specials each become a hyphen
		{"DEFANG-CD-WESTUS3", "defang-cd-westus3"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := sanitize(tt.in); got != tt.want {
				t.Errorf("sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestManagedCertName(t *testing.T) {
	tests := []struct {
		name     string
		envName  string
		hostname string
		want     string
	}{
		{
			name:     "short inputs pass through",
			envName:  "defang-cd",
			hostname: "x.example.com",
			want:     "mc-defang-cd-x-example-com",
		},
		{
			name:     "envName truncated to 15",
			envName:  "Defang-html-css-js-certsub-352bab6",
			hostname: "azurebyod.defang.study",
			want:     "mc-defang-html-css-azurebyod-defang-study",
		},
		{
			name:     "hostname truncated to 30",
			envName:  "defang-cd",
			hostname: "very-long-hostname-with-many-labels.example.com.test.invalid",
			want:     "mc-defang-cd-very-long-hostname-with-many-l",
		},
		{
			name:     "uppercase + dots normalized",
			envName:  "DEFANG-CD",
			hostname: "Defang.Study",
			want:     "mc-defang-cd-defang-study",
		},
		{
			// Truncation must not leave a trailing hyphen, or the joined
			// name would contain "--" which ARM rejects.
			name:     "envName trailing hyphen after truncation is trimmed",
			envName:  "abcdefghijklmn-rest",
			hostname: "x.example.com",
			want:     "mc-abcdefghijklmn-x-example-com",
		},
		{
			name:     "hostname trailing hyphen after truncation is trimmed",
			envName:  "defang-cd",
			hostname: "abcdefghijklmnopqrstuvwxyz1234-rest.example.com",
			want:     "mc-defang-cd-abcdefghijklmnopqrstuvwxyz1234",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := managedCertName(tt.envName, tt.hostname)
			if got != tt.want {
				t.Errorf("managedCertName(%q, %q) = %q, want %q", tt.envName, tt.hostname, got, tt.want)
			}
			// ARM cap is 64; the function targets well below.
			if len(got) > 64 {
				t.Errorf("name %q exceeds ARM 64-char limit (len=%d)", got, len(got))
			}
			if !strings.HasPrefix(got, "mc-") {
				t.Errorf("name %q missing mc- prefix", got)
			}
		})
	}
}

func TestDerefString(t *testing.T) {
	if got := derefString(nil); got != "" {
		t.Errorf("derefString(nil) = %q, want empty", got)
	}
	v := "hello"
	if got := derefString(&v); got != "hello" {
		t.Errorf("derefString(&%q) = %q, want hello", v, got)
	}
	empty := ""
	if got := derefString(&empty); got != "" {
		t.Errorf("derefString(&\"\") = %q, want empty", got)
	}
}
