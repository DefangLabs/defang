package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v4"
)

func TestVerifyToken(t *testing.T) {
	const testjwt = "eyJhbGciOiJQUzI1NiIsImtpZCI6IjJmZTk3ZWM2ODgxMjNhMzViZGI5ZGI4ZmM3M2MzYjhjNTNjNTZhZmIiLCJ0eXAiOiJKV1QifQ.eyJleHAiOjI2ODg1ODQyOTUsImh0dHBzOi8vZGVmYW5nLmlvL2p3dC9jbGFpbXMiOnsiZ2l0aHViLXVzZXJuYW1lIjoicmFwaGFlbHRtIn0sImh0dHBzOi8vaGFzdXJhLmlvL2p3dC9jbGFpbXMiOnsieC1oYXN1cmEtYWxsb3dlZC1yb2xlcyI6WyJ1c2VyIl0sIngtaGFzdXJhLWRlZmF1bHQtcm9sZSI6InVzZXIiLCJ4LWhhc3VyYS11c2VyLWlkIjoiMGMzNDU0NzgtMTkwOC00NDlkLWE1MzctOTI4ZDZhZjJjMDAxIn0sImlhdCI6MTY4ODU4NDIzNSwiaXNzIjoiaGVpbWRhbGwiLCJqdGkiOiI2NmJkM2MwZS00MzdiLTRjZjUtOWY5Mi0yNzAzNWVlNDlkZGEiLCJuYmYiOjE2ODg1ODQyMzUsInN1YiI6IjBjMzQ1NDc4LTE5MDgtNDQ5ZC1hNTM3LTkyOGQ2YWYyYzAwMSJ9.gDvzDfDKcO33FQWuy507KjRp3DPXz3v75EVGK3IHfvRuPn4Yx6hid8QzcjAd41KQ38hP8oZ4oKdVpfvSCCkkSbIGIqfPS-3stuIHqvmxzmzNufXODpm8BgX3xZ6FcCJ1wTwtzA2YIgee1Ufmt_zuJaJ4yCvXIy8QryZe93Lrz3gI_-2yh1k8KBjkBJXbC-Z8bOFkmfk9klOuSHk3BHwd90g2fKYtn0KiYA1vD168WAUwFIG1MvnNcLF1Bw-bzjgj9nwdhEbzNM802DVgj4ILvDi3q5o_Jc9MA07T3v1oFfQyS4w5SGMHdOwF6gwTfY4khsf_SsdHOeZZRC_Mz5Jp7g"

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(Jwks{
			Keys: []Jwk{
				{
					Kid: "2fe97ec688123a35bdb9db8fc73c3b8c53c56afb",
					Kty: "RSA",
					Use: "sig",
					N:   "u8kpMT9-bXHuVJTiVuYKvH1jQlsAD3Rq3K5Zn7DWKADOqPhmRQci4jUMv2tyScuRLnzShGw-86DFPIVMpoArruwG_j9iM94IVTx36567Kb0NtO1L4fAv_PGdwGVKWfVtlw2qB3cvoTIYFewKihYMPQpSaMxWONCuZ9f508UMYoEFWFBpcceyk5-3LgNstv1fcHLnDNOq9BGlQV6dx1mmUrB33YAZOcDVHIwAFKdhXdXoA80ZntjW1C7Litm7w3F3R0lmHtrvrswS9w23d5wZ0LfjDlxgR7rOzD1kxhKWFfpSxF5GSpw3c46agIeT5hi2lYLGuUAv-1K61fKa9zzBEw",
					E:   "AQAB",
				},
			},
		})
		if err != nil {
			panic(err)
		}
		w.Write(b)
	}))
	defer s.Close()

	tv := NewTokenVerifier(s.URL, "heimdall", []string{jwt.SigningMethodPS256.Name})

	t.Run("valid", func(t *testing.T) {
		sub, err := tv.VerifyAssertion(testjwt)
		if err != nil {
			t.Fatal(err)
		}
		if sub != "raphaeltm" {
			t.Error("expected subject: raphaeltm")
		}
	})

	t.Run("cache", func(t *testing.T) {
		tv.jwksURL = "http://localhost:1234" // invalid URL
		_, err := tv.VerifyAssertion(testjwt)
		if err != nil {
			t.Error("expected keys to be cached")
		}
	})
}
