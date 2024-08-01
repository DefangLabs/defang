package types

import "errors"

var ErrComposeFileNotFound = errors.New("no compose file found")
var ErrMultipleComposeFilesFound = errors.New(`multiple Compose files found: ["./compose.yaml" "./docker-compose.yml"]; use -f to specify which one to use`)
