## Build
```
go mod download
make
```

## Run
```
./defang
```

## Format Code
```
go fmt
```

## Update Dependencies
To regenerate the `go.mod` file:
```
go mod tidy
```

## Release
To release a new version, run:
```
make release
```
This will create a new tag (incrementing the patch number) and push it to the
repository, triggering a new build on the CI/CD pipeline.
