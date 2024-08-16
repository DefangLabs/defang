package http

import "net/url"

func RemoveQueryParam(qurl string) string {
	u, err := url.Parse(qurl)
	if err != nil {
		return qurl
	}
	u.RawQuery = ""
	return u.String()
}
