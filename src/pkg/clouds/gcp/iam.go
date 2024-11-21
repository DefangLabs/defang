package gcp

import (
	"context"
	"fmt"
	"log"
	"slices"

	artifactregistry "cloud.google.com/go/artifactregistry/apiv1"
	"cloud.google.com/go/iam"
	iamadm "cloud.google.com/go/iam/admin/apiv1"
	iamadmpb "cloud.google.com/go/iam/admin/apiv1/adminpb"
	"cloud.google.com/go/iam/apiv1/iampb"
	iamv1 "cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"cloud.google.com/go/storage"
	gax "github.com/googleapis/gax-go/v2"
)

func (gcp Gcp) EnsureRoleExists(ctx context.Context, roleId, title, description string, permissions []string) (string, error) {
	client, err := iamadm.NewIamClient(ctx)
	if err != nil {
		return "", fmt.Errorf("iam.NewIamClient: %w", err)
	}
	defer client.Close()

	// Check if the role already exists, if so, update it if necessary
	roleName := fmt.Sprintf("projects/%s/roles/%s", gcp.ProjectId, roleId)
	role, err := client.GetRole(ctx, &iamadmpb.GetRoleRequest{Name: roleName})
	if err == nil {
		existing := role.GetIncludedPermissions()
		slices.Sort(existing)
		slices.Sort(permissions)
		if slices.Equal(existing, permissions) &&
			role.GetTitle() == title &&
			role.GetDescription() == description &&
			role.Stage == iamadmpb.Role_GA {
			return role.Name, nil
		}

		role.IncludedPermissions = permissions
		role.Title = title
		role.Description = description
		role.Stage = iamadmpb.Role_GA
		if _, err := client.UpdateRole(ctx, &iamadmpb.UpdateRoleRequest{Role: role}); err != nil {
			return "", fmt.Errorf("failed to update role: %w", err)
		}
	}

	// If the role does not exist, create it
	if IsNotFound(err) {
		req := &iamadmpb.CreateRoleRequest{
			Parent: "projects/%s" + gcp.ProjectId,
			RoleId: roleId,
			Role: &iamadmpb.Role{
				Title:               title,
				Description:         description,
				IncludedPermissions: permissions,
				Stage:               iamadmpb.Role_GA, // TODO: investigate stage
			},
		}
		role, err := client.CreateRole(ctx, req)
		if err != nil {
			return "", fmt.Errorf("failed to create role: %w", err)
		}

		log.Printf("Role %s created successfully.", roleId)
		return role.Name, nil
	}

	// Handle unexpected errors
	return "", fmt.Errorf("failed to check role existence: %w", err)
}

func (gcp Gcp) EnsureServiceAccountExists(ctx context.Context, serviceAccountId, displayName, description string) (string, error) {
	client, err := iamadm.NewIamClient(ctx)
	if err != nil {
		return "", fmt.Errorf("iam.NewIamClient: %w", err)
	}
	defer client.Close()

	// Check if the service account already exists, if so, update it if necessary
	serviceAccountEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", serviceAccountId, gcp.ProjectId)
	serviceAccountPath := fmt.Sprintf("projects/%s/serviceAccounts/%s", gcp.ProjectId, serviceAccountEmail)
	account, err := client.GetServiceAccount(ctx, &iamadmpb.GetServiceAccountRequest{Name: serviceAccountPath})
	if err == nil {
		if account.GetDisplayName() == displayName &&
			account.GetDescription() == description {
			return account.Name, nil
		}

		account.DisplayName = displayName
		account.Description = description
		if _, err := client.UpdateServiceAccount(ctx, &iamadmpb.ServiceAccount{Name: account.Name, DisplayName: displayName, Description: description}); err != nil {
			return "", fmt.Errorf("failed to update service account: %w", err)
		}
	}

	// If the service account does not exist, create it
	if IsNotFound(err) {
		req := &iamadmpb.CreateServiceAccountRequest{
			AccountId: serviceAccountId,
			ServiceAccount: &iamadmpb.ServiceAccount{
				DisplayName: displayName,
				Description: description,
			},
			Name: "projects/%s" + gcp.ProjectId,
		}
		account, err := client.CreateServiceAccount(ctx, req)
		if err != nil {
			return "", fmt.Errorf("failed to create service account: %w", err)
		}

		log.Printf("Service account %s created successfully.", serviceAccountId)
		return account.Name, nil
	}

	// Handle unexpected errors
	return "", fmt.Errorf("failed to check service account existence: %w", err)
}

func (gcp Gcp) EnsureServiceAccountHasRoles(ctx context.Context, serviceAccount string, roles []string) error {
	client, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create resource manager client: %w", err)
	}
	defer client.Close()

	projectResource := "projects/%s" + gcp.ProjectId
	return ensureServiceAccountHasRolesWithResource(ctx, client, projectResource, serviceAccount, roles)
}

func (gcp Gcp) EnsureServiceAccountHasBucketRoles(ctx context.Context, bucketName, serviceAccount string, roles []string) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer client.Close()

	// Get the bucket's IAM policy
	bucket := client.Bucket(bucketName)
	serviceAccountMember := "serviceAccount:%s" + serviceAccount
	policy, err := bucket.IAM().Policy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for bucket %s: %w", bucketName, err)
	}

	needUpdate := false
	for _, roleStr := range roles {
		role := iam.RoleName(roleStr)
		memebers := policy.Members(role)
		if !slices.Contains(memebers, serviceAccountMember) {
			policy.Add(serviceAccountMember, role)
			needUpdate = true
		}
	}

	if !needUpdate {
		return nil
	}

	if err := bucket.IAM().SetPolicy(ctx, policy); err != nil {
		return fmt.Errorf("failed to set IAM policy for bucket %s: %w", bucketName, err)
	}
	return nil
}

func (gcp Gcp) EnsureServiceAccountHasArtifactRegistryRoles(ctx context.Context, repo, serviceAccount string, roles []string) error {
	client, err := artifactregistry.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create artifact registry client: %w", err)
	}
	defer client.Close()

	return ensureServiceAccountHasRolesWithResource(ctx, client, repo, serviceAccount, roles)
}

type resourceWithIAMPolicyClient interface {
	GetIamPolicy(context.Context, *iampb.GetIamPolicyRequest, ...gax.CallOption) (*iamv1.Policy, error)
	SetIamPolicy(context.Context, *iampb.SetIamPolicyRequest, ...gax.CallOption) (*iamv1.Policy, error)
}

func ensureServiceAccountHasRolesWithResource(ctx context.Context, client resourceWithIAMPolicyClient, resource, serviceAccount string, roles []string) error {
	serviceAccountMember := "serviceAccount:%s"+ serviceAccount
	policy, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for resource %s: %w", resource, err)
	}

	var rolesFound []string
	bindingNeedsUpdate := false
	for _, binding := range policy.Bindings {
		if slices.Contains(roles, binding.Role) {
			rolesFound = append(rolesFound, binding.Role)
			if slices.Contains(binding.Members, serviceAccountMember) {
				continue
			}
			binding.Members = append(binding.Members, serviceAccountMember)
			bindingNeedsUpdate = true
		}
	}

	var rolesNotFound []string
	for _, role := range roles {
		if !slices.Contains(rolesFound, role) {
			rolesNotFound = append(rolesNotFound, role)
			policy.Bindings = append(policy.Bindings, &iamv1.Binding{
				Role:    role,
				Members: []string{serviceAccountMember},
			})
		}
	}

	if !bindingNeedsUpdate && len(rolesNotFound) == 0 {
		return nil
	}
	if _, err := client.SetIamPolicy(ctx, &iampb.SetIamPolicyRequest{Resource: resource, Policy: policy}); err != nil {
		return fmt.Errorf("failed to set IAM policy for resource %s: %w", resource, err)
	}
	return nil
}
