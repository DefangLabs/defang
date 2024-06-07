package pkg

import (
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
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

func Contains[T comparable](s []T, v []T) bool {
	// Convert both arrays to maps for efficient lookup
	map1 := make(map[T]bool)
	map2 := make(map[T]bool)

	for _, val := range s {
		map1[val] = true
	}

	for _, val := range v {
		map2[val] = true
	}

	// Check if each element of map2 is in map1
	for val := range map2 {
		if !map1[val] {
			return false
		}
	}

	// If all elements of map2 are in map1, return true
	return true
}
