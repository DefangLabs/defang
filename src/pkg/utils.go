package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	validServiceRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)      // alphanumeric+hyphens 1 ≤ len < 64
	validSecretRegex  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`) // alphanumeric+underscore 1 ≤ len ≤ 64
)

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

func PostForValues(_url, contentType string, body io.Reader) (url.Values, error) {
	resp, err := http.Post(_url, contentType, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// FIXME: on error, the body might not be URL-encoded
	values, err := url.ParseQuery(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse response body %s: %w", resp.Status, err)
	}
	return values, nil
}

func SplitByComma(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

type OneOrList []string

func (l *OneOrList) UnmarshalJSON(data []byte) error {
	ls := []string{}
	if err := json.Unmarshal(data, &ls); err != nil {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		ls = []string{s}
	}
	*l = ls
	return nil
}

func RandomID() string {
	const uint64msb = 1 << 63 // always set the MSB to ensure we get ≥12 digits
	return strconv.FormatUint(rand.Uint64()|uint64msb, 36)[1:]
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
