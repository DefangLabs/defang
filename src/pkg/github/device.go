package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/defang-io/defang/src/pkg/http"
)

func StartDeviceFlow(ctx context.Context, clientId string) (url.Values, error) {
	codeUrl := "https://github.com/login/device/code?client_id=" + clientId
	q, err := http.PostForValues(codeUrl, "application/json", nil)
	if err != nil {
		return nil, err
	}

	interval, err := strconv.Atoi(q.Get("interval"))
	if err != nil {
		return nil, err
	}

	fmt.Printf("Please visit %s and enter the code %s\n", q.Get("verification_uri"), q.Get("user_code"))

	values := url.Values{
		"client_id":   {clientId},
		"device_code": q["device_code"],
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	accessTokenUrl := "https://github.com/login/oauth/access_token?" + values.Encode()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		time.Sleep(time.Duration(interval) * time.Second)

		q, err := http.PostForValues(accessTokenUrl, "application/json", nil)
		if err != nil || q.Get("error") != "" {
			switch q.Get("error") {
			case "authorization_pending":
				continue
			case "slow_down":
				if interval, err = strconv.Atoi(q.Get("interval")); err == nil {
					continue
				}
			}
			return nil, fmt.Errorf("%w: %v", err, q.Get("error_description"))
		}

		return q, nil
	}
}
