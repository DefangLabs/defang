package compose

import (
	"regexp"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
)

// Based on cd/aws/defang_service.ts
var healthcheckUrlRegex = regexp.MustCompile(`(?i)(?:http:\/\/)?(?:localhost|127\.0\.0\.1)(?::(\d{1,5}))?([?/](?:[?/a-z0-9._~!$&()*+,;=:@-]|%[a-f0-9]{2}){0,333})?`)

func GetHealthCheckPathAndPort(hc *types.HealthCheckConfig) (string, int) {
	path := "/"
	port := 80
	if hc == nil || len(hc.Test) < 1 || (hc.Test[0] != "CMD" && hc.Test[0] != "CMD-SHELL") {
		return path, port
	}
	for _, arg := range hc.Test[1:] {
		if match := healthcheckUrlRegex.FindStringSubmatch(arg); match != nil {
			if match[1] != "" {
				if n, err := strconv.Atoi(match[1]); err == nil {
					port = n
				}
			}
			if match[2] != "" {
				path = match[2]
			}
		}
	}
	return path, port
}
