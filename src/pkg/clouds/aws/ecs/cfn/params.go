package cfn

const (
	ParamsCIRoleName              = "CIRoleName"              // Name of the CI IAM role (optional)
	ParamsDockerHubAccessToken    = "DockerHubAccessToken"    // Access token for Docker Hub authentication (optional)
	ParamsDockerHubUsername       = "DockerHubUsername"       // Username for Docker Hub authentication (optional)
	ParamsEnablePullThroughCache  = "EnablePullThroughCache"  // "true"/"false" - Whether to enable ECR pull-through cache
	ParamsExistingVpcId           = "ExistingVpcId"           // VPC ID string or empty to create new VPC
	ParamsOidcProviderAudiences   = "OidcProviderAudiences"   // Comma-delimited list of OIDC provider trusted audiences (optional)
	ParamsOidcProviderClaims      = "OidcProviderClaims"      // Comma-delimited list of additional OIDC claim conditions as JSON "key":"value" pairs (optional)
	ParamsOidcProviderIssuer      = "OidcProviderIssuer"      // OIDC provider trusted issuer (optional)
	ParamsOidcProviderSubjects    = "OidcProviderSubjects"    // Comma-delimited list of OIDC provider trusted subject patterns (optional)
	ParamsOidcProviderThumbprints = "OidcProviderThumbprints" // Comma-delimited list of OIDC provider thumbprints (optional)
	ParamsRetainBucket            = "RetainBucket"            // "true"/"false" - Whether to retain S3 bucket on stack deletion
)
