package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	artifactregistry "cloud.google.com/go/artifactregistry/apiv1"
	"cloud.google.com/go/iam"
	iamadm "cloud.google.com/go/iam/admin/apiv1"
	iamadmpb "cloud.google.com/go/iam/admin/apiv1/adminpb"
	"cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"cloud.google.com/go/storage"
	"github.com/DefangLabs/defang/src/pkg"
	gax "github.com/googleapis/gax-go/v2"
)

func (gcp Gcp) EnsureRoleExists(ctx context.Context, roleId, title, description string, permissions []string) (string, error) {
	client, err := iamadm.NewIamClient(ctx, gcp.Options...)
	if err != nil {
		return "", fmt.Errorf("unable to ensure role exists, failed to create Iam client: %w", err)
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
			slog.Debug("Role already exists", "roleId", roleId)
			return role.Name, nil
		}

		// Update the role
		role.IncludedPermissions = permissions
		role.Title = title
		role.Description = description
		role.Stage = iamadmpb.Role_GA
		slog.InfoContext(ctx, "Updating role "+roleId)
		if _, err := client.UpdateRole(ctx, &iamadmpb.UpdateRoleRequest{Name: roleName, Role: role}); err != nil {
			return "", fmt.Errorf("failed to update role: %w", err)
		}
	}

	// If the role does not exist, create it
	if IsNotFound(err) {
		req := &iamadmpb.CreateRoleRequest{
			Parent: "projects/" + gcp.ProjectId,
			RoleId: roleId,
			Role: &iamadmpb.Role{
				Title:               title,
				Description:         description,
				IncludedPermissions: permissions,
				Stage:               iamadmpb.Role_GA, // TODO: investigate stage
			},
		}
		slog.InfoContext(ctx, "Creating role "+roleId)
		role, err = client.CreateRole(ctx, req)
		if err != nil {
			return "", fmt.Errorf("failed to create role: %w", err)
		}
		slog.Debug("Role created successfully", "roleId", roleId)
	}

	// Wait for the role to be created or updated
	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		role, err = client.GetRole(ctx, &iamadmpb.GetRoleRequest{Name: role.Name})
		if err != nil {
			if IsNotFound(err) {
				if err := pkg.SleepWithContext(ctx, 3*time.Second); err != nil {
					return "", err
				}
				continue
			}
			return "", fmt.Errorf("failed to verify role creation: %w", err)
		}
		return role.Name, nil
	}
	return "", fmt.Errorf("timed out waiting for creation of role %s", roleId)
}

func (gcp Gcp) EnsureServiceAccountExists(ctx context.Context, serviceAccountId, displayName, description string) (string, error) {
	client, err := iamadm.NewIamClient(ctx, gcp.Options...)
	if err != nil {
		return "", fmt.Errorf("unable to ensure Service Account exists, failed to create Iam Client: %w", err)
	}
	defer client.Close()

	// Check if the service account already exists, if so, update it if necessary
	serviceAccountEmail := gcp.GetServiceAccountEmail(serviceAccountId)
	serviceAccountPath := fmt.Sprintf("projects/%s/serviceAccounts/%s", gcp.ProjectId, serviceAccountEmail)
	account, err := client.GetServiceAccount(ctx, &iamadmpb.GetServiceAccountRequest{Name: serviceAccountPath})
	if err == nil {
		if account.GetDisplayName() == displayName &&
			account.GetDescription() == description {
			slog.Debug("Service account already exists", "serviceAccountId", serviceAccountId)
			return account.Name, nil
		}

		account.DisplayName = displayName
		account.Description = description
		slog.InfoContext(ctx, "Updating service account "+serviceAccountId)
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
			Name: "projects/" + gcp.ProjectId,
		}
		slog.InfoContext(ctx, "Creating service account "+serviceAccountId)
		account, err := client.CreateServiceAccount(ctx, req)
		if err != nil {
			return "", fmt.Errorf("failed to create service account: %w", err)
		}

		slog.Debug("Service account created successfully", "serviceAccountId", serviceAccountId)
		accountName := account.Name
		for start := time.Now(); time.Since(start) < 5*time.Minute; {
			account, err = client.GetServiceAccount(ctx, &iamadmpb.GetServiceAccountRequest{Name: accountName})
			if err != nil {
				if IsNotFound(err) {
					if err := pkg.SleepWithContext(ctx, 3*time.Second); err != nil {
						return "", err
					}
					continue
				}
				return "", fmt.Errorf("failed to verify service account creation: %w", err)
			}
			return account.Name, nil
		}
		return "", fmt.Errorf("timed out waiting for creation of service account %s", serviceAccountId)
	}

	// Handle unexpected errors
	return "", fmt.Errorf("failed to check service account existence: %w", err)
}

func (gcp Gcp) EnsurePrincipalHasRoles(ctx context.Context, serviceAccount string, roles []string) error {
	client, err := resourcemanager.NewProjectsClient(ctx, gcp.Options...)
	if err != nil {
		return fmt.Errorf("failed to create resource manager client: %w", err)
	}
	defer client.Close()

	projectResource := "projects/" + gcp.ProjectId
	return ensurePrincipalHasRolesWithResource(ctx, client, projectResource, serviceAccount, roles)
}

func (gcp Gcp) EnsurePrincipalHasBucketRoles(ctx context.Context, bucketName, principal string, roles []string) error {
	client, err := storage.NewClient(ctx, gcp.Options...)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer client.Close()

	// Get the bucket's IAM policy
	bucket := client.Bucket(bucketName)
	policy, err := bucket.IAM().Policy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for bucket %s: %w", bucketName, err)
	}

	needUpdate := false
	for _, roleStr := range roles {
		role := iam.RoleName(roleStr)
		members := policy.Members(role)
		if !slices.Contains(members, principal) {
			policy.Add(principal, role)
			needUpdate = true
		}
	}

	if !needUpdate {
		slog.Debug("Principal already has roles on bucket", "principal", principal, "roles", roles, "bucket", bucketName)
		return nil
	}

	slog.InfoContext(ctx, fmt.Sprintf("Updating IAM policy for principal %s on bucket %s", principal, bucketName))
	for i := range maxAttempts { // Service account might not be visible for a few seconds after creation for policy attachment
		if err := bucket.IAM().SetPolicy(ctx, policy); err != nil {
			if i < maxAttempts-1 {
				slog.InfoContext(ctx, fmt.Sprintf("Failed to set IAM policy, will retry in %v: %v\n", retryInterval, err))
				if err := pkg.SleepWithContext(ctx, retryInterval); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("failed to set IAM policy for bucket %s: %w", bucketName, err)
		} else {
			break
		}
	}

checkPolicy:
	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		vp, err := bucket.IAM().Policy(ctx)
		if err != nil {
			return fmt.Errorf("failed to verify IAM policy for principal %s on bucket %s: %w", principal, bucketName, err)
		}

		for _, roleStr := range roles {
			role := iam.RoleName(roleStr)
			members := vp.Members(role)
			if !slices.Contains(members, principal) {
				if err := pkg.SleepWithContext(ctx, 3*time.Second); err != nil {
					return err
				}
				continue checkPolicy
			}
		}
		return nil
	}
	return fmt.Errorf("timed out waiting for IAM policy update on bucket %s", bucketName)
}

func (gcp Gcp) EnsureServiceAccountHasArtifactRegistryRoles(ctx context.Context, repo, serviceAccount string, roles []string) error {
	client, err := artifactregistry.NewClient(ctx, gcp.Options...)
	if err != nil {
		return fmt.Errorf("unable to ensure service account artifact registry role, failed to create artifact registry client: %w", err)
	}
	defer client.Close()

	return ensurePrincipalHasRolesWithResource(ctx, client, repo, serviceAccount, roles)
}

// TODO: Investigate if this can be merged with EnsureServiceAccountHasRoles
func (gcp Gcp) EnsurePrincipalHasServiceAccountRoles(ctx context.Context, principal, serviceAccount string, roles []string) error {
	client, err := iamadm.NewIamClient(ctx, gcp.Options...)
	if err != nil {
		return fmt.Errorf("unable to ensure user service account role, failed to create iam client: %w", err)
	}
	defer client.Close()

	resource := fmt.Sprintf("projects/%s/serviceAccounts/%s", gcp.ProjectId, serviceAccount)

	policy, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for service account %s: %w", serviceAccount, err)
	}

	needUpdate := false
	for _, roleStr := range roles {
		role := iam.RoleName(roleStr)
		members := policy.Members(role)
		if !slices.Contains(members, principal) {
			policy.Add(principal, role)
			needUpdate = true
		}
	}

	if !needUpdate {
		return nil
	}

	slog.InfoContext(ctx, fmt.Sprintf("Updating IAM policy for %s on service account %s", principal, serviceAccount))
	for i := range maxAttempts { // Service account might not be visible for a few seconds after creation for policy attachment
		if _, err := client.SetIamPolicy(ctx, &iamadm.SetIamPolicyRequest{
			Resource: resource,
			Policy:   policy,
		}); err != nil {
			if i < maxAttempts-1 {
				slog.InfoContext(ctx, fmt.Sprintf("Failed to set IAM policy for service account %s, will retry in %v: %v\n", serviceAccount, retryInterval, err))
				if err := pkg.SleepWithContext(ctx, retryInterval); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("failed to set IAM policy for service account %s: %w", serviceAccount, err)
		} else {
			break
		}
	}

checkPolicy:
	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		vp, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
		if err != nil {
			return fmt.Errorf("failed to verify IAM policy for user %v on service account %s: %w", principal, serviceAccount, err)
		}
		for _, roleStr := range roles {
			role := iam.RoleName(roleStr)
			members := vp.Members(role)
			if !slices.Contains(members, principal) {
				if err := pkg.SleepWithContext(ctx, 3*time.Second); err != nil {
					return err
				}
				continue checkPolicy
			}
		}
		return nil
	}
	return fmt.Errorf("timed out waiting for IAM policy update on service account %s", serviceAccount)
}

type resourceWithIAMPolicyClient interface {
	GetIamPolicy(context.Context, *iampb.GetIamPolicyRequest, ...gax.CallOption) (*iampb.Policy, error)
	SetIamPolicy(context.Context, *iampb.SetIamPolicyRequest, ...gax.CallOption) (*iampb.Policy, error)
}

func ensurePrincipalHasRolesWithResource(ctx context.Context, client resourceWithIAMPolicyClient, resource, principal string, roles []string) error {
	policy, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for resource %s: %w", resource, err)
	}

	var rolesFound []string
	bindingNeedsUpdate := false
	for _, binding := range policy.Bindings {
		if slices.Contains(roles, binding.Role) {
			rolesFound = append(rolesFound, binding.Role)
			if slices.Contains(binding.Members, principal) {
				continue
			}
			binding.Members = append(binding.Members, principal)
			bindingNeedsUpdate = true
		}
	}

	var rolesNotFound []string
	for _, role := range roles {
		if !slices.Contains(rolesFound, role) {
			rolesNotFound = append(rolesNotFound, role)
			policy.Bindings = append(policy.Bindings, &iampb.Binding{
				Role:    role,
				Members: []string{principal},
			})
		}
	}

	if !bindingNeedsUpdate && len(rolesNotFound) == 0 {
		slog.Debug("Principal already has roles on resource", "principal", principal, "roles", roles, "resource", resource)
		return nil
	}
	slog.InfoContext(ctx, "Updating IAM policy for resource "+resource)

	for i := range maxAttempts { // Service account might not be visible for a few seconds after creation for policy attachment
		if _, err := client.SetIamPolicy(ctx, &iampb.SetIamPolicyRequest{Resource: resource, Policy: policy}); err != nil {
			if i < maxAttempts-1 {
				slog.Debug("Failed to set IAM policy for resource, will retry", "resource", resource, "retryInterval", retryInterval, "error", err)
				if err := pkg.SleepWithContext(ctx, retryInterval); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("failed to set IAM policy for resource %s: %w", resource, err)
		} else {
			break
		}
	}

	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		vp, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
		if err != nil {
			return fmt.Errorf("failed to verify IAM policy for resource %s: %w", resource, err)
		}
		var rolesSet []string
		for _, binding := range vp.Bindings {
			if slices.Contains(roles, binding.Role) && slices.Contains(binding.Members, principal) {
				rolesSet = append(rolesSet, binding.Role)
			}
		}
		if len(rolesSet) < len(roles) {
			if err := pkg.SleepWithContext(ctx, 3*time.Second); err != nil {
				return err
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("timed out waiting for IAM policy update on resource %s", resource)
}

func (gcp Gcp) GetServiceAccountEmail(name string) string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", name, gcp.ProjectId)
}
