package gcp

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"github.com/DefangLabs/defang/src/pkg"
	"google.golang.org/api/iterator"
)

type ProjectId string

const (
	suffixLen             = 1 + 12 // "-" + 12 (pkg.RandomID)
	maxProjectIDLen       = 30
	maxProjectIDPrefixLen = maxProjectIDLen - suffixLen
)

// A project ID has the following requirements:
//   - Must be 6 to 30 characters in length.
//   - Can only contain lowercase letters, numbers, and hyphens.
//   - Must start with a letter.
//   - Cannot end with a hyphen.
//   - Cannot be in use or previously used; this includes deleted projects.
//   - Cannot contain restricted strings, such as google, null, undefined, and ssl.
//
// Compose Project Name:
//   - must contain only lowercase letters, decimal digits, dashes, and underscores,
//   - and must begin with a lowercase letter or decimal digit
//
// Differences:
//   - Project ID cannot contain underscores
//   - Project ID cannot start with a digit
//   - Project ID cannot end with a hyphen
//   - Project ID cannot contain restricted strings: google, null, undefined, ssl
//   - Note: console test shows only google is restricted
//   - Project ID must be 6 ~ 30 characters
//   - Project ID must be unique including deleted projects
func ProjectIDFromName(name string) ProjectId {
	// Sanity step: Only lowercase letters
	id := strings.ToLower(name)
	// Sanity step: Remove any illegal characters
	id = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(id, "")
	// Project ID cannot contain underscores
	id = strings.ReplaceAll(id, "_", "-")
	// Project ID cannot start with a digit
	if id[0] >= '0' && id[0] <= '9' {
		id = "p-" + id
	}
	// Project ID cannot end with a hyphen
	id = strings.TrimSuffix(id, "-")
	// Project ID cannot contain restricted strings: google, null, undefined, ssl
	id = regexp.MustCompile(`(google|null|undefined|ssl)`).ReplaceAllString(id, "")
	// Project ID must be unique including deleted projects
	suffix := "-" + pkg.RandomID()
	// Project ID must be 6 ~ 30 characters
	if len(id) > maxProjectIDPrefixLen {
		id = id[:maxProjectIDPrefixLen]
	}
	return ProjectId(id + suffix)
}

func (id ProjectId) String() string {
	return string(id)
}

func (id ProjectId) IsValid() bool {
	if len(id) < suffixLen {
		return false
	}
	if id[len(id)-suffixLen] != '-' {
		return false
	}
	if !pkg.IsValidRandomID(string(id[len(id)-suffixLen+1:])) {
		return false
	}
	return true
}

func (id ProjectId) Prefix() string {
	if !id.IsValid() {
		return string(id)
	}
	return string(id)[:len(id)-suffixLen]
}

func (id ProjectId) Suffix() string {
	if !id.IsValid() {
		return ""
	}
	return string(id[len(id)-suffixLen:])
}

type Gcp struct {
	Region    string
	ProjectId string
}

func (gcp Gcp) GetProjectID() ProjectId {
	return ProjectId(gcp.ProjectId)
}

func (gcp Gcp) EnsureProjectExists(ctx context.Context, projectName string) (*resourcemanagerpb.Project, error) {
	client, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to ensure project exists, failed to create project client: %w", err)
	}
	defer client.Close()

	var project *resourcemanagerpb.Project
	// Find if there is already a CD project with the defang-cd prefix
	projectId := ProjectIDFromName(projectName)
	req := &resourcemanagerpb.SearchProjectsRequest{}

	// TODO:: Figure out how to use client.ListProjects to find projects without an org as according
	// to doc Search projects is eventually consistent, so 2 consecutive calls may create 2 cd projects
	// as the first one is still not visible to the search API call.
	it := client.SearchProjects(ctx, req)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("unable to ensure project exists, failed getting next iteration: %w", err)
		}
		id := ProjectId(resp.ProjectId)
		if resp.State != resourcemanagerpb.Project_ACTIVE {
			continue
		}
		if id.Prefix() == projectId.Prefix() {
			project = resp
			break
		}
	}

	if project == nil {
		// return nil, fmt.Errorf("project not found")
		// If project doesn't exist, create it
		createReq := &resourcemanagerpb.CreateProjectRequest{
			Project: &resourcemanagerpb.Project{
				ProjectId:   projectId.String(),
				DisplayName: projectName,
			},
		}
		projectOp, err := client.CreateProject(ctx, createReq)
		if err != nil {
			return nil, fmt.Errorf("failed to create project: %w", err)
		}
		project, err = projectOp.Wait(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to wait for projectOp: %w", err)
		}
	}

	return project, nil
}
