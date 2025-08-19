package cli

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
)

func CreateRandomConfigValue() string {
	// Note that no error handling is necessary, as Read always succeeds.
	key := make([]byte, 32)
	rand.Read(key)
	str := base64.StdEncoding.EncodeToString(key)
	re := regexp.MustCompile("[+/=]")
	str = re.ReplaceAllString(str, "")
	return str
}
