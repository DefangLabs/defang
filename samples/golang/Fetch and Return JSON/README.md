# Go HTTP Server with Fiscal Data API

This is a simple HTTP server written in Go that serves two endpoints: / and /rates. The / endpoint responds with a JSON object containing the status, while the /rates endpoint fetches data from the Fiscal Data Treasury API and returns the response to the client.