package compose

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// DockerfileValidationError represents an error found during Dockerfile validation
type DockerfileValidationError struct {
	ServiceName    string
	DockerfilePath string
	Line           int
	Message        string
	Err            error
}

func (e *DockerfileValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("service %q: Dockerfile validation error in %q at line %d: %s",
			e.ServiceName, e.DockerfilePath, e.Line, e.Message)
	}
	return fmt.Sprintf("service %q: Dockerfile validation error in %q: %s",
		e.ServiceName, e.DockerfilePath, e.Message)
}

func (e *DockerfileValidationError) Unwrap() error {
	return e.Err
}

// ValidateDockerfile validates the syntax and basic structure of a Dockerfile
func ValidateDockerfile(dockerfilePath string, serviceName string) error {
	term.Debugf("Validating Dockerfile: %s for service %q", dockerfilePath, serviceName)

	// Read the Dockerfile
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return &DockerfileValidationError{
			ServiceName:    serviceName,
			DockerfilePath: dockerfilePath,
			Message:        fmt.Sprintf("failed to read Dockerfile: %v", err),
			Err:            err,
		}
	}

	// Parse the Dockerfile using buildkit's parser
	result, err := parser.Parse(bytes.NewReader(content))
	if err != nil {
		// Check if it's an empty file error
		errMsg := err.Error()
		if strings.Contains(errMsg, "file with no instructions") {
			return &DockerfileValidationError{
				ServiceName:    serviceName,
				DockerfilePath: dockerfilePath,
				Message:        "Dockerfile is empty or contains only comments",
				Err:            err,
			}
		}
		return &DockerfileValidationError{
			ServiceName:    serviceName,
			DockerfilePath: dockerfilePath,
			Message:        fmt.Sprintf("syntax error: %v", err),
			Err:            err,
		}
	}

	// Check if Dockerfile is empty
	if result.AST == nil || len(result.AST.Children) == 0 {
		return &DockerfileValidationError{
			ServiceName:    serviceName,
			DockerfilePath: dockerfilePath,
			Message:        "Dockerfile is empty or contains only comments",
		}
	}

	// Check for FROM instruction (required in valid Dockerfiles)
	hasFrom := false
	var fromLine int
	for _, child := range result.AST.Children {
		if strings.ToUpper(child.Value) == "FROM" {
			hasFrom = true
			fromLine = child.StartLine
			break
		}
	}

	if !hasFrom {
		return &DockerfileValidationError{
			ServiceName:    serviceName,
			DockerfilePath: dockerfilePath,
			Message:        "Dockerfile must contain at least one FROM instruction",
		}
	}

	// Check if FROM is the first non-comment, non-ARG instruction
	for _, child := range result.AST.Children {
		instruction := strings.ToUpper(child.Value)
		if instruction != "ARG" && instruction != "FROM" {
			if child.StartLine < fromLine {
				return &DockerfileValidationError{
					ServiceName:    serviceName,
					DockerfilePath: dockerfilePath,
					Line:           child.StartLine,
					Message:        fmt.Sprintf("%s instruction must come after FROM (except ARG)", instruction),
				}
			}
			break
		}
	}

	// Check for parser warnings
	if len(result.Warnings) > 0 {
		var warnings []string
		for _, warning := range result.Warnings {
			if warning.Location != nil {
				warnings = append(warnings, fmt.Sprintf("line %d: %s", warning.Location.Start.Line, warning.Short))
			} else {
				warnings = append(warnings, warning.Short)
			}
		}
		// Log warnings but don't fail validation
		term.Warnf("service %q: Dockerfile %q has warnings:\n  %s", serviceName, dockerfilePath, strings.Join(warnings, "\n  "))
	}

	return nil
}

// ValidateServiceDockerfiles validates all Dockerfiles referenced by services in a project
func ValidateServiceDockerfiles(project *Project) error {
	var errors []error

	for _, service := range project.Services {
		// Skip services without build context
		if service.Build == nil {
			continue
		}

		// Skip if using Railpack (indicated by special marker)
		if service.Build.Dockerfile == RAILPACK {
			continue
		}

		// Determine the Dockerfile path
		dockerfilePath := service.Build.Dockerfile
		if dockerfilePath == "" {
			dockerfilePath = "Dockerfile"
		}

		// Make it absolute relative to the build context
		if !filepath.IsAbs(dockerfilePath) {
			dockerfilePath = filepath.Join(service.Build.Context, dockerfilePath)
		}

		// Check if file exists
		if _, err := os.Stat(dockerfilePath); err != nil {
			if os.IsNotExist(err) {
				// This might be handled later by Railpack or may be a remote context
				// Only validate if the file exists
				term.Debugf("Skipping validation for service %q: Dockerfile %q does not exist", service.Name, dockerfilePath)
				continue
			}
			errors = append(errors, &DockerfileValidationError{
				ServiceName:    service.Name,
				DockerfilePath: dockerfilePath,
				Message:        fmt.Sprintf("failed to access Dockerfile: %v", err),
				Err:            err,
			})
			continue
		}

		// Validate the Dockerfile
		if err := ValidateDockerfile(dockerfilePath, service.Name); err != nil {
			errors = append(errors, err)
		}
	}

	// Return a combined error if there were any validation errors
	if len(errors) > 0 {
		return &DockerfileValidationErrors{Errors: errors}
	}

	return nil
}

// DockerfileValidationErrors represents multiple Dockerfile validation errors
type DockerfileValidationErrors struct {
	Errors []error
}

func (e *DockerfileValidationErrors) Error() string {
	var messages []string
	messages = append(messages, "Dockerfile validation failed:")
	for _, err := range e.Errors {
		messages = append(messages, "  • "+err.Error())
	}
	return strings.Join(messages, "\n")
}

func (e *DockerfileValidationErrors) Unwrap() []error {
	return e.Errors
}
