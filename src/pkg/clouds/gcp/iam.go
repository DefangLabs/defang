package gcp

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	artifactregistry "cloud.google.com/go/artifactregistry/apiv1"
	"cloud.google.com/go/iam"
	iamadm "cloud.google.com/go/iam/admin/apiv1"
	iamadmpb "cloud.google.com/go/iam/admin/apiv1/adminpb"
	"cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"cloud.google.com/go/storage"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	gax "github.com/googleapis/gax-go/v2"
)

func (gcp Gcp) EnsureRoleExists(ctx context.Context, roleId, title, description string, permissions []string) (string, error) {
	client, err := iamadm.NewIamClient(ctx)
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
			term.Debugf("Role %s already exists", roleId)
			return role.Name, nil
		}

		// Update the role
		role.IncludedPermissions = permissions
		role.Title = title
		role.Description = description
		role.Stage = iamadmpb.Role_GA
		term.Infof("Updating role %s", roleId)
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
		term.Infof("Creating role %s", roleId)
		role, err = client.CreateRole(ctx, req)
		if err != nil {
			return "", fmt.Errorf("failed to create role: %w", err)
		}
		term.Debugf("Role %s created successfully.", roleId)
	}

	// Wait for the role to be created or updated
	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		role, err = client.GetRole(ctx, &iamadmpb.GetRoleRequest{Name: role.Name})
		if err != nil {
			if IsNotFound(err) {
				pkg.SleepWithContext(ctx, 3*time.Second)
				continue
			}
			return "", fmt.Errorf("failed to verify role creation: %w", err)
		}
		return role.Name, nil
	}
	return "", fmt.Errorf("timed out waiting for creation of role %s", roleId)
}

func (gcp Gcp) EnsureServiceAccountExists(ctx context.Context, serviceAccountId, displayName, description string) (string, error) {
	client, err := iamadm.NewIamClient(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to ensure Service Account exists, failed to create Iam Client: %w", err)
	}
	defer client.Close()

	// Check if the service account already exists, if so, update it if necessary
	serviceAccountEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", serviceAccountId, gcp.ProjectId)
	serviceAccountPath := fmt.Sprintf("projects/%s/serviceAccounts/%s", gcp.ProjectId, serviceAccountEmail)
	account, err := client.GetServiceAccount(ctx, &iamadmpb.GetServiceAccountRequest{Name: serviceAccountPath})
	if err == nil {
		if account.GetDisplayName() == displayName &&
			account.GetDescription() == description {
			term.Debugf("Service account %s already exists", serviceAccountId)
			return account.Name, nil
		}

		account.DisplayName = displayName
		account.Description = description
		term.Infof("Updating service account %s", serviceAccountId)
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
		term.Infof("Creating service account %s", serviceAccountId)
		account, err := client.CreateServiceAccount(ctx, req)
		if err != nil {
			return "", fmt.Errorf("failed to create service account: %w", err)
		}

		term.Debugf("Service account %s created successfully.", serviceAccountId)
		accountName := account.Name
		for start := time.Now(); time.Since(start) < 5*time.Minute; {
			account, err = client.GetServiceAccount(ctx, &iamadmpb.GetServiceAccountRequest{Name: accountName})
			if err != nil {
				if IsNotFound(err) {
					pkg.SleepWithContext(ctx, 3*time.Second)
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

func (gcp Gcp) EnsureServiceAccountHasRoles(ctx context.Context, serviceAccount string, roles []string) error {
	client, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create resource manager client: %w", err)
	}
	defer client.Close()

	projectResource := "projects/" + gcp.ProjectId
	return ensureAccountHasRolesWithResource(ctx, client, projectResource, serviceAccount, roles)
}

func (gcp Gcp) EnsureServiceAccountHasBucketRoles(ctx context.Context, bucketName, serviceAccount string, roles []string) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer client.Close()

	// Get the bucket's IAM policy
	bucket := client.Bucket(bucketName)
	serviceAccountMember := "serviceAccount:" + serviceAccount
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
		term.Debugf("Service account %s already has roles %v on bucket %s", serviceAccount, roles, bucketName)
		return nil
	}

	term.Infof("Updating IAM policy for service account %s on bucket %s", serviceAccount, bucketName)
	for i := range 3 { // Service account might not be visible for a few seconds after creation for policy attachment
		if err := bucket.IAM().SetPolicy(ctx, policy); err != nil {
			if i < 2 {
				term.Infof("Failed to set IAM policy, will retry in 5s: %v\n", err)
				pkg.SleepWithContext(ctx, 5*time.Second)
				continue
			}
			return fmt.Errorf("failed to set IAM policy for bucket %s: %w", bucketName, err)
		} else {
			break
		}
	}

	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		vp, err := bucket.IAM().Policy(ctx)
		if err != nil {
			return fmt.Errorf("failed to verify IAM policy for service account %s on bucket %s: %w", serviceAccount, bucketName, err)
		}

		for _, roleStr := range roles {
			role := iam.RoleName(roleStr)
			memebers := vp.Members(role)
			if !slices.Contains(memebers, serviceAccountMember) {
				pkg.SleepWithContext(ctx, 3*time.Second)
				continue
			}
		}
		return nil
	}
	return fmt.Errorf("timed out waiting for IAM policy update on bucket %s", bucketName)
}

func (gcp Gcp) EnsureServiceAccountHasArtifactRegistryRoles(ctx context.Context, repo, serviceAccount string, roles []string) error {
	client, err := artifactregistry.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("unable to ensure service account artifact registry role, failed to create artifact registry client: %w", err)
	}
	defer client.Close()

	return ensureAccountHasRolesWithResource(ctx, client, repo, serviceAccount, roles)
}

// TODO: Investigate if this can be merged with EnsureServiceAccountHasRoles
func (gcp Gcp) EnsureUserHasServiceAccountRoles(ctx context.Context, user, serviceAccount string, roles []string) error {
	client, err := iamadm.NewIamClient(ctx)
	if err != nil {
		log.Fatalf("unable to ensure user service account role, failed to create artifact registry client: %v", err)
	}
	defer client.Close()

	resource := fmt.Sprintf("projects/%s/serviceAccounts/%s", gcp.ProjectId, serviceAccount)
	member := "user:" + user

	policy, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for service account %s: %w", serviceAccount, err)
	}

	needUpdate := false
	for _, roleStr := range roles {
		role := iam.RoleName(roleStr)
		memebers := policy.Members(role)
		if !slices.Contains(memebers, member) {
			policy.Add(member, role)
			needUpdate = true
		}
	}

	if !needUpdate {
		return nil
	}

	term.Infof("Updating IAM policy for %s on service account %s", user, serviceAccount)
	if _, err := client.SetIamPolicy(ctx, &iamadm.SetIamPolicyRequest{
		Resource: resource,
		Policy:   policy,
	}); err != nil {
		return fmt.Errorf("failed to set IAM policy for service account %s: %w", serviceAccount, err)
	}

	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		vp, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
		if err != nil {
			return fmt.Errorf("failed to verify IAM policy for user %v on service account %s: %w", user, serviceAccount, err)
		}
		for _, roleStr := range roles {
			role := iam.RoleName(roleStr)
			memebers := vp.Members(role)
			if !slices.Contains(memebers, member) {
				pkg.SleepWithContext(ctx, 3*time.Second)
				continue
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

func ensureAccountHasRolesWithResource(ctx context.Context, client resourceWithIAMPolicyClient, resource, account string, roles []string) error {
	var member string
	if strings.HasSuffix(account, ".gserviceaccount.com") {
		member = "serviceAccount:" + account
	} else {
		member = "user:" + account
	}
	policy, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for resource %s: %w", resource, err)
	}

	var rolesFound []string
	bindingNeedsUpdate := false
	for _, binding := range policy.Bindings {
		if slices.Contains(roles, binding.Role) {
			rolesFound = append(rolesFound, binding.Role)
			if slices.Contains(binding.Members, member) {
				continue
			}
			binding.Members = append(binding.Members, member)
			bindingNeedsUpdate = true
		}
	}

	var rolesNotFound []string
	for _, role := range roles {
		if !slices.Contains(rolesFound, role) {
			rolesNotFound = append(rolesNotFound, role)
			policy.Bindings = append(policy.Bindings, &iampb.Binding{
				Role:    role,
				Members: []string{member},
			})
		}
	}

	if !bindingNeedsUpdate && len(rolesNotFound) == 0 {
		term.Debugf("%s already has roles %v on resource %s", member, roles, resource)
		return nil
	}
	term.Infof("Updating IAM policy for resource %s", resource)
	if _, err := client.SetIamPolicy(ctx, &iampb.SetIamPolicyRequest{Resource: resource, Policy: policy}); err != nil {
		return fmt.Errorf("failed to set IAM policy for resource %s: %w", resource, err)
	}

	for start := time.Now(); time.Since(start) < 5*time.Minute; {
		vp, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{Resource: resource})
		if err != nil {
			return fmt.Errorf("failed to verify IAM policy for resource %s: %w", resource, err)
		}
		var rolesSet []string
		for _, binding := range vp.Bindings {
			if slices.Contains(roles, binding.Role) && slices.Contains(binding.Members, member) {
				rolesSet = append(rolesSet, binding.Role)
			}
		}
		if len(rolesSet) < len(roles) {
			pkg.SleepWithContext(ctx, 3*time.Second)
			continue
		}
		return nil
	}
	return fmt.Errorf("timed out waiting for IAM policy update on resource %s", resource)
}
