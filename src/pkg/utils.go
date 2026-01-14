package pkg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/pmezard/go-difflib/difflib"
)

var (
	validServiceRegex = regexp.MustCompile(`^[A-Za-z0-9]+([-_][A-Za-z0-9]+)*$`) // alphanumeric+hyphens 1 ≤ len < 64
	validSecretRegex  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)     // alphanumeric+underscore 1 ≤ len ≤ 64

	// Standard GCP environment variables in order of precedence; https://registry.terraform.io/providers/hashicorp/google/5.10.0/docs/guides/provider_reference#project-1
	GCPProjectEnvVars = []string{"GCP_PROJECT_ID", "GOOGLE_PROJECT", "GOOGLE_CLOUD_PROJECT", "GCLOUD_PROJECT", "CLOUDSDK_CORE_PROJECT"}
	GCPRegionEnvVars  = []string{"GCP_LOCATION", "GOOGLE_REGION", "GCLOUD_REGION", "CLOUDSDK_COMPUTE_REGION"}
)

func GetCurrentUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" { // Windows
		return user
	}
	return "unknown"
}

func IsValidServiceName(name string) bool {
	return len(name) < 20 && validServiceRegex.MatchString(name) // HACK to avoid long target group names
}

func IsValidTailName(name string) bool {
	return len(name) < 64 && validServiceRegex.MatchString(name)
}

func IsValidSecretName(name string) bool {
	return validSecretRegex.MatchString(name)
}

// Getenv returns the value of an environment variable; defaults to the fallback string.
func Getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// GetenvBool returns the boolean value of an environment variable; defaults to false.
func GetenvBool(key string) bool {
	val, _ := strconv.ParseBool(os.Getenv(key))
	return val
}

func GetFirstEnv(keys ...string) (string, string) {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			return key, value
		}
	}
	return "", ""
}

func SplitByComma(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func RandomIndex(n int) int {
	if n <= 0 {
		panic("n must be greater than 0")
	}
	// #nosec G404 - crypto is not important here, we just need a random index
	return rand.Intn(n)
}

func RandomID() string {
	const uint64msb = 1 << 63 // always set the MSB to ensure we get 13 digits
	// #nosec G404 - this is not a security-sensitive ID, just a random identifier
	return strconv.FormatUint(rand.Uint64()|uint64msb, 36)[1:] // first digit only has 2 bits of entropy, so skip it
}

func IsValidRandomID(s string) bool {
	_, err := strconv.ParseUint(s, 36, 64)
	return len(s) == 12 && err == nil
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func IsDirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func SleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func SubscriptionTierToString(tier defangv1.SubscriptionTier) string {
	switch tier {
	case defangv1.SubscriptionTier_SUBSCRIPTION_TIER_UNSPECIFIED:
		fallthrough // free tier
	case defangv1.SubscriptionTier_HOBBY:
		return "Hobby"
	case defangv1.SubscriptionTier_PERSONAL:
		return "Personal"
	case defangv1.SubscriptionTier_PRO:
		return "Pro"
	case defangv1.SubscriptionTier_TEAM:
		return "Team"
	default:
		return "Unknown"
	}
}

func Ensure(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

func IsValidTime(t time.Time) bool {
	// When converting a timestamppb to a time.Time, the zero/nil value becomes 1970-01-01 00:00:00 UTC,
	// and because of timezones this can either be sometime on 1969-12-31 or on 1970-01-01 in local time.
	// We could be even more conservative and check for > 2000 or so, but this is more predictable.
	return t.Year() > 1970
}

func Compare(actual []byte, goldenFile string) error {
	// Replace the absolute path in context to make the golden file portable
	absPath, _ := filepath.Abs(goldenFile)
	actual = bytes.ReplaceAll(actual, []byte(filepath.Dir(absPath)), []byte{'.'})

	golden, err := os.ReadFile(goldenFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read golden file: %w", err)
		}
		return os.WriteFile(goldenFile, actual, 0644)
	} else {
		if err := Diff(string(actual), string(golden)); err != nil {
			return fmt.Errorf("%s %w", goldenFile, err)
		}
	}
	return nil
}

func Diff(actualRaw, goldenRaw string) error {
	if actualRaw == goldenRaw {
		return nil
	}

	// Show the diff (but only the lines that differ to avoid overwhelming output)
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(goldenRaw),
		B:        difflib.SplitLines(actualRaw),
		FromFile: "Expected",
		FromDate: "",
		ToFile:   "Actual",
		ToDate:   "",
		Context:  1,
	})

	return fmt.Errorf("mismatch:\n%s", diff)
}

var shellSpecialChars = regexp.MustCompile(`[^\w@%+=:,./-]`) // copied from al.essio.dev/pkg/shellescape

// ShellQuote returns a shell-quoted string of the given arguments.
// When needed, arguments are quoted with double quotes, so that spaces and env vars are preserved.
func ShellQuote(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if shellSpecialChars.MatchString(arg) {
			arg = strconv.Quote(arg)
		}
		quoted[i] = arg
	}
	return strings.Join(quoted, " ")
}

func AwsInEnv() string {
	env, _ := GetFirstEnv("AWS_PROFILE", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_ROLE_ARN")
	return env
}

func DoInEnv() string {
	env, _ := GetFirstEnv("DIGITALOCEAN_ACCESS_TOKEN", "DIGITALOCEAN_TOKEN")
	return env
}

func GcpInEnv() string {
	env, _ := GetFirstEnv(GCPProjectEnvVars...)
	return env
}
