package cfn

import (
	"crypto/sha256"
	"encoding/hex"

	awscodebuild "github.com/DefangLabs/defang/src/pkg/clouds/aws/codebuild"
	"github.com/aws/smithy-go/ptr"
	"github.com/awslabs/goformation/v7/cloudformation"
	"github.com/awslabs/goformation/v7/cloudformation/codebuild"
	"github.com/awslabs/goformation/v7/cloudformation/ecr"
	"github.com/awslabs/goformation/v7/cloudformation/iam"
	"github.com/awslabs/goformation/v7/cloudformation/logs"
	"github.com/awslabs/goformation/v7/cloudformation/policies"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
	"github.com/awslabs/goformation/v7/cloudformation/secretsmanager"
	"github.com/awslabs/goformation/v7/cloudformation/tags"
)

const (
	maxCachePrefixLength = 20 // prefix must be 2-20 characters long; should be 30 https://github.com/hashicorp/terraform-provider-aws/pull/34716

	TagKeyCreatedBy   = "defang:CreatedBy"
	TagKeyManagedBy   = "defang:ManagedBy"
	TagKeyPrefix      = "defang:Prefix"
	TagKeyStackName   = "defang:CloudFormationStackName"
	TagKeyStackRegion = "defang:CloudFormationStackRegion"
)

func GetCacheRepoPrefix(prefix, suffix string) string {
	repo := prefix + suffix
	if len(repo) > maxCachePrefixLength {
		// Cache repo name is too long; hash it and use the first 6 chars
		hash := sha256.Sum256([]byte(prefix))
		return hex.EncodeToString(hash[:])[:6] + "-" + suffix
	}
	return repo
}

const TemplateRevision = 4 // bump this when the template changes!

// CreateTemplate creates a parameterized CloudFormation template for the CD infrastructure.
// Uses CodeBuild instead of ECS for running Pulumi deployments.
func CreateTemplate(stack string) (*cloudformation.Template, error) {
	const oidcProviderDefaultAud = "sts.amazonaws.com"

	prefix := stack + "-"

	defaultTags := []tags.Tag{
		{
			Key:   TagKeyCreatedBy,
			Value: awscodebuild.CrunProjectName,
		},
		{
			Key:   TagKeyPrefix,
			Value: stack,
		},
		{
			Key:   TagKeyManagedBy,
			Value: "CloudFormation",
		},
		{
			Key:   TagKeyStackName,
			Value: cloudformation.Ref("AWS::StackName"),
		},
		{
			Key:   TagKeyStackRegion,
			Value: cloudformation.Ref("AWS::Region"),
		},
	}

	template := cloudformation.NewTemplate()
	template.Description = "Defang AWS CloudFormation template for the CD task. Do not delete this stack in the AWS console: use the Defang CLI instead. To create this stack, scroll down to acknowledge the risks and press 'Create stack'."

	// Parameters
	template.Parameters[ParamsRetainBucket] = cloudformation.Parameter{
		Type:          "String",
		Default:       ptr.String("true"),
		AllowedValues: []any{"true", "false"},
		Description:   ptr.String("Whether to retain the S3 bucket on stack deletion"),
	}
	template.Parameters[ParamsEnablePullThroughCache] = cloudformation.Parameter{
		Type:          "String",
		Default:       ptr.String("true"),
		AllowedValues: []any{"true", "false"},
		Description:   ptr.String("Whether to enable ECR pull-through cache"),
	}
	template.Parameters[ParamsDockerHubUsername] = cloudformation.Parameter{
		Type:        "String",
		Default:     ptr.String(""),
		Description: ptr.String("Docker Hub username for private registry access (optional)"),
	}
	template.Parameters[ParamsDockerHubAccessToken] = cloudformation.Parameter{
		Type:        "String",
		Default:     ptr.String(""),
		Description: ptr.String("Docker Hub access token for private registry access (optional)"),
		NoEcho:      ptr.Bool(true),
	}
	template.Parameters[ParamsOidcProviderIssuer] = cloudformation.Parameter{
		Type:        "String",
		Default:     ptr.String(""),
		Description: ptr.String("OIDC provider trusted issuer (optional)"),
	}
	template.Parameters[ParamsOidcProviderSubjects] = cloudformation.Parameter{
		Type:        "CommaDelimitedList",
		Default:     ptr.String(""),
		Description: ptr.String("OIDC provider trusted subject pattern(s) (optional)"),
	}
	template.Parameters[ParamsOidcProviderThumbprints] = cloudformation.Parameter{
		Type:        "CommaDelimitedList",
		Default:     ptr.String(""),
		Description: ptr.String("OIDC provider thumbprint(s) (optional)"),
	}
	template.Parameters[ParamsCIRoleName] = cloudformation.Parameter{
		Type:        "String",
		Default:     ptr.String(""),
		Description: ptr.String("Name of the CI role (optional)"),
	}
	template.Parameters[ParamsOidcProviderAudiences] = cloudformation.Parameter{
		Type:        "CommaDelimitedList",
		Default:     ptr.String(oidcProviderDefaultAud),
		Description: ptr.String("OIDC provider trusted audience(s) (optional)"),
	}
	template.Parameters[ParamsOidcProviderClaims] = cloudformation.Parameter{
		Type:        "CommaDelimitedList",
		Default:     ptr.String(""),
		Description: ptr.String(`Additional OIDC claim conditions as comma-separated JSON "key":"value" pairs (optional)`),
	}

	// Metadata - AWS::CloudFormation::Interface for parameter grouping and labels
	template.Metadata = map[string]interface{}{
		"AWS::CloudFormation::Interface": map[string]interface{}{
			"ParameterGroups": []map[string]interface{}{
				{
					"Label":      map[string]string{"default": "CI/CD Integration (OIDC)"},
					"Parameters": []string{ParamsOidcProviderIssuer, ParamsOidcProviderSubjects, ParamsOidcProviderAudiences, ParamsCIRoleName, ParamsOidcProviderThumbprints, ParamsOidcProviderClaims},
				},
				{
					"Label":      map[string]string{"default": "Container Registry (ECR Pull-Through Cache)"},
					"Parameters": []string{ParamsEnablePullThroughCache, ParamsDockerHubUsername, ParamsDockerHubAccessToken},
				},
				{
					"Label":      map[string]string{"default": "Storage Configuration"},
					"Parameters": []string{ParamsRetainBucket},
				},
			},
			"ParameterLabels": map[string]interface{}{
				ParamsRetainBucket:            map[string]string{"default": "Retain S3 Bucket on Delete"},
				ParamsEnablePullThroughCache:  map[string]string{"default": "Enable ECR Pull-Through Cache"},
				ParamsDockerHubUsername:       map[string]string{"default": "Docker Hub Username"},
				ParamsDockerHubAccessToken:    map[string]string{"default": "Docker Hub Access Token"},
				ParamsOidcProviderIssuer:      map[string]string{"default": "OIDC Provider Issuer URL"},
				ParamsOidcProviderSubjects:    map[string]string{"default": "OIDC Trusted Subject Patterns"},
				ParamsOidcProviderAudiences:   map[string]string{"default": "OIDC Trusted Audiences"},
				ParamsOidcProviderThumbprints: map[string]string{"default": "OIDC Provider Thumbprints"},
				ParamsOidcProviderClaims:      map[string]string{"default": "Additional OIDC Claim Conditions"},
				ParamsCIRoleName:              map[string]string{"default": "CI Role Name"},
			},
		},
	}

	// Conditions
	const _condRetainS3Bucket = "RetainS3Bucket"
	template.Conditions[_condRetainS3Bucket] = cloudformation.Equals(cloudformation.Ref(ParamsRetainBucket), "true")
	const _condEnablePullThroughCache = "EnablePullThroughCache"
	template.Conditions[_condEnablePullThroughCache] = cloudformation.Equals(cloudformation.Ref(ParamsEnablePullThroughCache), "true")
	const _condEnableDockerPullThroughCache = "EnableDockerPullThroughCache"
	template.Conditions[_condEnableDockerPullThroughCache] = cloudformation.And([]string{
		cloudformation.Equals(cloudformation.Ref(ParamsEnablePullThroughCache), "true"),
		cloudformation.Not([]string{cloudformation.Equals(cloudformation.Ref(ParamsDockerHubUsername), "")}),
		cloudformation.Not([]string{cloudformation.Equals(cloudformation.Ref(ParamsDockerHubAccessToken), "")}),
	})
	const _condOidcProvider = "OidcProvider"
	template.Conditions[_condOidcProvider] = cloudformation.And([]string{
		cloudformation.Not([]string{cloudformation.Equals(cloudformation.Ref(ParamsOidcProviderIssuer), "")}),
		cloudformation.Not([]string{cloudformation.Equals(cloudformation.Join("", cloudformation.Ref(ParamsOidcProviderSubjects)), "")}),
	})
	const _condOverrideCIRoleName = "OverrideCIRoleName"
	template.Conditions[_condOverrideCIRoleName] = cloudformation.Not([]string{cloudformation.Equals(cloudformation.Ref(ParamsCIRoleName), "")})
	const _condOidcClaims = "OidcClaims"
	template.Conditions[_condOidcClaims] = cloudformation.Not([]string{cloudformation.Equals(cloudformation.Join("", cloudformation.Ref(ParamsOidcProviderClaims)), "")})
	const _condOidcThumbprints = "OidcThumbprints"
	template.Conditions[_condOidcThumbprints] = cloudformation.Not([]string{cloudformation.Equals(cloudformation.Join("", cloudformation.Ref(ParamsOidcProviderThumbprints)), "")})

	// 1. S3 bucket (for deployment state)
	const _bucket = "Bucket"
	template.Resources[_bucket] = &s3.Bucket{
		Tags:                            defaultTags,
		AWSCloudFormationDeletionPolicy: policies.DeletionPolicy(cloudformation.If(_condRetainS3Bucket, "RetainExceptOnCreate", "Delete")),
		VersioningConfiguration: &s3.Bucket_VersioningConfiguration{
			Status: "Enabled",
		},
	}

	// 2. CloudWatch log group
	const _logGroup = "LogGroup"
	template.Resources[_logGroup] = &logs.LogGroup{
		Tags:            defaultTags,
		RetentionInDays: ptr.Int(1),
	}

	// 3. ECR pull-through cache rules
	ecrPublicPrefix := GetCacheRepoPrefix(prefix, "ecr-public")
	dockerPublicPrefix := GetCacheRepoPrefix(prefix, "docker-public")

	const _pullThroughCache = "PullThroughCache"
	template.Resources[_pullThroughCache] = &ecr.PullThroughCacheRule{
		AWSCloudFormationCondition: _condEnablePullThroughCache,
		EcrRepositoryPrefix:        ptr.String(ecrPublicPrefix),
		UpstreamRegistryUrl:        ptr.String(awscodebuild.EcrPublicRegistry),
	}

	// #nosec G101 - not a secret
	const _privateRepoSecret = "PrivateRepoSecret"
	template.Resources[_privateRepoSecret] = &secretsmanager.Secret{
		AWSCloudFormationCondition: _condEnableDockerPullThroughCache,
		Tags:                       defaultTags,
		Description:                ptr.String("Docker Hub credentials for the ECR pull-through cache rule"),
		Name:                       ptr.String("ecr-pullthroughcache/" + dockerPublicPrefix),
		SecretString:               ptr.String(cloudformation.Sub(`{"username":"${` + ParamsDockerHubUsername + `}","accessToken":"${` + ParamsDockerHubAccessToken + `}"}`)),
	}

	const _pullThroughCacheDocker = "PullThroughCacheDocker"
	template.Resources[_pullThroughCacheDocker] = &ecr.PullThroughCacheRule{
		AWSCloudFormationCondition: _condEnableDockerPullThroughCache,
		EcrRepositoryPrefix:        ptr.String(dockerPublicPrefix),
		UpstreamRegistryUrl:        ptr.String("registry-1.docker.io"),
		CredentialArn:              cloudformation.RefPtr(_privateRepoSecret),
	}

	// 4. CodeBuild service role
	const _codeBuildServiceRole = "CodeBuildServiceRole"
	template.Resources[_codeBuildServiceRole] = &iam.Role{
		Tags: defaultTags,
		AssumeRolePolicyDocument: map[string]any{
			"Version": "2012-10-17",
			"Statement": []map[string]any{
				{
					"Effect": "Allow",
					"Principal": map[string]any{
						"Service": "codebuild.amazonaws.com",
					},
					"Action": "sts:AssumeRole",
				},
			},
		},
		ManagedPolicyArns: []string{
			"arn:aws:iam::aws:policy/AdministratorAccess", // TODO: restrict
		},
	}

	// 5. CodeBuild project (CFN does not prefix CodeBuild project names with the stack name)
	const _codeBuildProject = "DefangCD"
	template.Resources[_codeBuildProject] = &codebuild.Project{
		Tags: defaultTags,
		Source: &codebuild.Project_Source{
			Type:      "NO_SOURCE",
			BuildSpec: ptr.String("version: 0.2\nphases:\n  build:\n    commands:\n      - echo 'buildspec should be overridden at StartBuild time'\n"),
		},
		Artifacts: &codebuild.Project_Artifacts{
			Type: "NO_ARTIFACTS",
		},
		Cache: &codebuild.Project_ProjectCache{
			Type:  "LOCAL",
			Modes: []string{"LOCAL_DOCKER_LAYER_CACHE"},
		},
		Environment: &codebuild.Project_Environment{
			ComputeType: "BUILD_GENERAL1_MEDIUM",
			Type:        "LINUX_CONTAINER",
			Image:       "aws/codebuild/amazonlinux2-x86_64-standard:5.0", // placeholder; overridden at StartBuild time
		},
		ServiceRole: cloudformation.Ref(_codeBuildServiceRole),
		LogsConfig: &codebuild.Project_LogsConfig{
			CloudWatchLogs: &codebuild.Project_CloudWatchLogsConfig{
				Status:    "ENABLED",
				GroupName: cloudformation.RefPtr(_logGroup),
			},
		},
	}

	// 6. IAM OIDC provider
	const _oidcProvider = "OIDCProvider"
	template.Resources[_oidcProvider] = &OIDCProvider{
		AWSCloudFormationCondition: _condOidcProvider,
		Tags:                       defaultTags,
		ClientIdList:               cloudformation.Ref(ParamsOidcProviderAudiences),
		ThumbprintList: cloudformation.If(_condOidcThumbprints,
			cloudformation.Ref(ParamsOidcProviderThumbprints),
			cloudformation.Ref("AWS::NoValue"),
		),
		Url: cloudformation.SubPtr(`https://${` + ParamsOidcProviderIssuer + `}`),
	}

	// 7. CI role
	const _CIRole = "CIRole"
	template.Resources[_CIRole] = &iam.Role{
		AWSCloudFormationCondition: _condOidcProvider,
		RoleName: cloudformation.IfPtr(_condOverrideCIRoleName,
			cloudformation.Ref(ParamsCIRoleName),
			cloudformation.Ref("AWS::NoValue"),
		),
		Tags: defaultTags,
		AssumeRolePolicyDocument: cloudformation.SubVars(`{
    "Version": "2012-10-17",
    "Statement": [{
        "Effect": "Allow",
        "Principal": {
            "Federated": "${Provider}"
        },
        "Action": "sts:AssumeRoleWithWebIdentity",
        "Condition": {
            "StringEquals": {
                "${`+ParamsOidcProviderIssuer+`}:aud": [ "${Audiences}" ]${ExtraClaims}
            },
            "StringLike": {
                "${`+ParamsOidcProviderIssuer+`}:sub": [ "${Subjects}" ]
            }
        }
    }]
}`, map[string]any{
			"Audiences":   cloudformation.Join(`","`, cloudformation.Ref(ParamsOidcProviderAudiences)),
			"Provider":    cloudformation.Ref(_oidcProvider),
			"Subjects":    cloudformation.Join(`","`, cloudformation.Ref(ParamsOidcProviderSubjects)),
			"ExtraClaims": cloudformation.If(_condOidcClaims, cloudformation.Join("", []any{",", cloudformation.Join(",", cloudformation.Ref(ParamsOidcProviderClaims))}), ""),
		}),
		ManagedPolicyArns: []string{
			"arn:aws:iam::aws:policy/AdministratorAccess",
		},
	}

	// Outputs
	template.Outputs[OutputsCIRoleARN] = cloudformation.Output{
		Condition:   ptr.String(_condOidcProvider),
		Description: ptr.String("ARN of the CI role"),
		Value:       cloudformation.GetAtt(_CIRole, "Arn"),
	}
	template.Outputs[OutputsLogGroupARN] = cloudformation.Output{
		Description: ptr.String("ARN of the CloudWatch log group"),
		Value:       cloudformation.GetAtt(_logGroup, "Arn"),
	}
	template.Outputs[OutputsBucketName] = cloudformation.Output{
		Description: ptr.String("Name of the S3 bucket"),
		Value:       cloudformation.Ref(_bucket),
	}
	template.Outputs[OutputsCodeBuildProjectName] = cloudformation.Output{
		Description: ptr.String("Name of the CodeBuild project"),
		Value:       cloudformation.Ref(_codeBuildProject),
	}
	template.Outputs[OutputsEcrCachePrefix] = cloudformation.Output{
		Condition:   ptr.String(_condEnablePullThroughCache),
		Description: ptr.String("ECR pull-through cache prefix for public ECR"),
		Value:       ecrPublicPrefix,
	}
	template.Outputs[OutputsDockerCachePrefix] = cloudformation.Output{
		Condition:   ptr.String(_condEnableDockerPullThroughCache),
		Description: ptr.String("ECR pull-through cache prefix for Docker Hub"),
		Value:       dockerPublicPrefix,
	}
	template.Outputs[OutputsTemplateVersion] = cloudformation.Output{
		Description: ptr.String("Version of this CloudFormation template"),
		Value:       cloudformation.Int(TemplateRevision),
	}

	return template, nil
}
