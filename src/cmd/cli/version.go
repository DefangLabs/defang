package main // this must be "main" or -ldflags will fail to set the version

var version = "development" // overwritten by build script -ldflags "-X main.version=..." and GoReleaser
