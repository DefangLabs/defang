package tools

import (
	"context"
	"iter"
	"os"
	"strings"
	"testing"
	"time"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	defangv1connect "github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Mock implementations
type mockProviderCreator struct {
	mock.Mock
}

func (m *mockProviderCreator) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient, stack string) cliClient.Provider {
	args := m.Called(ctx, providerId, client, stack)
	provider, ok := args.Get(0).(cliClient.Provider)
	if !ok {
		return nil
	}
	return provider
}

type mockElicitationsController struct {
	mock.Mock
}

func (m *mockElicitationsController) RequestString(ctx context.Context, message, field string) (string, error) {
	args := m.Called(ctx, message, field)
	return args.String(0), args.Error(1)
}

func (m *mockElicitationsController) RequestStringWithDefault(ctx context.Context, message, field, defaultValue string) (string, error) {
	args := m.Called(ctx, message, field, defaultValue)
	return args.String(0), args.Error(1)
}

func (m *mockElicitationsController) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	args := m.Called(ctx, message, field, options)
	return args.String(0), args.Error(1)
}

type mockFabricClient struct {
	mock.Mock
}

func (m *mockFabricClient) ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error) {
	args := m.Called(ctx, req)
	resp, ok := args.Get(0).(*defangv1.ListDeploymentsResponse)
	if !ok {
		return nil, args.Error(1)
	}
	return resp, args.Error(1)
}

// We only need to implement ListDeployments for our tests, so we'll embed the interface
// and only override the method we care about
func (m *mockFabricClient) AgreeToS(context.Context) error { return nil }
func (m *mockFabricClient) CanIUse(context.Context, *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) CheckLoginAndToS(context.Context) error { return nil }
func (m *mockFabricClient) Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) DelegateSubdomainZone(context.Context, *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) DeleteSubdomainZone(context.Context, *defangv1.DeleteSubdomainZoneRequest) error {
	return nil
}
func (m *mockFabricClient) Estimate(context.Context, *defangv1.EstimateRequest) (*defangv1.EstimateResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) GenerateCompose(context.Context, *defangv1.GenerateComposeRequest) (*defangv1.GenerateComposeResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) GenerateFiles(context.Context, *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) GetController() defangv1connect.FabricControllerClient { return nil }
func (m *mockFabricClient) GetDelegateSubdomainZone(context.Context, *defangv1.GetDelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) GetPlaygroundProjectDomain(context.Context) (*defangv1.GetPlaygroundProjectDomainResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) GetSelectedProvider(context.Context, *defangv1.GetSelectedProviderRequest) (*defangv1.GetSelectedProviderResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) GetTenantName() types.TenantName                        { return "" }
func (m *mockFabricClient) GetVersions(context.Context) (*defangv1.Version, error) { return nil, nil }
func (m *mockFabricClient) Preview(context.Context, *defangv1.PreviewRequest) (*defangv1.PreviewResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) Publish(context.Context, *defangv1.PublishRequest) error { return nil }
func (m *mockFabricClient) PutDeployment(context.Context, *defangv1.PutDeploymentRequest) error {
	return nil
}
func (m *mockFabricClient) RevokeToken(context.Context) error { return nil }
func (m *mockFabricClient) SetSelectedProvider(context.Context, *defangv1.SetSelectedProviderRequest) error {
	return nil
}
func (m *mockFabricClient) Token(context.Context, *defangv1.TokenRequest) (*defangv1.TokenResponse, error) {
	return nil, nil
}
func (m *mockFabricClient) Track(string, ...cliClient.Property) error { return nil }
func (m *mockFabricClient) VerifyDNSSetup(context.Context, *defangv1.VerifyDNSSetupRequest) error {
	return nil
}
func (m *mockFabricClient) WhoAmI(context.Context) (*defangv1.WhoAmIResponse, error) { return nil, nil }

type mockStacksManager struct {
	mock.Mock
}

func (m *mockStacksManager) Create(params stacks.StackParameters) (string, error) {
	args := m.Called(params)
	return args.String(0), args.Error(1)
}

func (m *mockStacksManager) Read(stackName string) (*stacks.StackParameters, error) {
	args := m.Called(stackName)
	param, ok := args.Get(0).(*stacks.StackParameters)
	if !ok {
		return nil, args.Error(1)
	}
	return param, args.Error(1)
}

func (m *mockStacksManager) LoadParameters(params *stacks.StackParameters) {}

func (m *mockStacksManager) List() ([]stacks.StackListItem, error) {
	args := m.Called()
	list, ok := args.Get(0).([]stacks.StackListItem)
	if !ok {
		return nil, args.Error(1)
	}
	return list, args.Error(1)
}

type mockProvider struct {
	mock.Mock
}

// Implement DNSResolver interface
func (m *mockProvider) ServicePrivateDNS(name string) string                    { return "" }
func (m *mockProvider) ServicePublicDNS(name string, projectName string) string { return "" }
func (m *mockProvider) UpdateShardDomain(ctx context.Context) error             { return nil }

// Implement Provider interface
func (m *mockProvider) AccountInfo(context.Context) (*cliClient.AccountInfo, error) { return nil, nil }
func (m *mockProvider) BootstrapCommand(context.Context, cliClient.BootstrapCommandRequest) (types.ETag, error) {
	return "", nil
}
func (m *mockProvider) BootstrapList(context.Context, bool) (iter.Seq[string], error) {
	return nil, nil
}
func (m *mockProvider) CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return nil, nil
}
func (m *mockProvider) DelayBeforeRetry(context.Context) error { return nil }
func (m *mockProvider) Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	return nil, nil
}
func (m *mockProvider) DeleteConfig(context.Context, *defangv1.Secrets) error { return nil }
func (m *mockProvider) Deploy(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return nil, nil
}
func (m *mockProvider) Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error) {
	return "", nil
}
func (m *mockProvider) GetDeploymentStatus(context.Context) error { return nil }
func (m *mockProvider) GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error) {
	return nil, nil
}
func (m *mockProvider) GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	return nil, nil
}
func (m *mockProvider) GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return nil, nil
}
func (m *mockProvider) ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return nil, nil
}
func (m *mockProvider) PrepareDomainDelegation(context.Context, cliClient.PrepareDomainDelegationRequest) (*cliClient.PrepareDomainDelegationResponse, error) {
	return nil, nil
}
func (m *mockProvider) Preview(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return nil, nil
}
func (m *mockProvider) PutConfig(context.Context, *defangv1.PutConfigRequest) error { return nil }
func (m *mockProvider) QueryForDebug(context.Context, *defangv1.DebugRequest) error { return nil }
func (m *mockProvider) QueryLogs(context.Context, *defangv1.TailRequest) (cliClient.ServerStream[defangv1.TailResponse], error) {
	return nil, nil
}
func (m *mockProvider) Subscribe(context.Context, *defangv1.SubscribeRequest) (cliClient.ServerStream[defangv1.SubscribeResponse], error) {
	return nil, nil
}
func (m *mockProvider) TearDown(context.Context) error                    { return nil }
func (m *mockProvider) RemoteProjectName(context.Context) (string, error) { return "", nil }
func (m *mockProvider) SetCanIUseConfig(*defangv1.CanIUseResponse)        {}
func (m *mockProvider) SetUpCD(context.Context) error                     { return nil }
func (m *mockProvider) TearDownCD(context.Context) error                  { return nil }

// Helper function to create a test stack parameters
func createTestStackParameters(name string, provider cliClient.ProviderID, region string) *stacks.StackParameters {
	return &stacks.StackParameters{
		Name:     name,
		Provider: provider,
		Region:   region,
	}
}

// Helper function to create deployment response
func createDeploymentResponse(stackName, provider, region string, deployedAt time.Time) *defangv1.ListDeploymentsResponse {
	var providerEnum defangv1.Provider
	if provider == "defang" {
		providerEnum = defangv1.Provider_DEFANG
	} else {
		providerEnum = defangv1.Provider(defangv1.Provider_value[strings.ToUpper(provider)])
	}

	return &defangv1.ListDeploymentsResponse{
		Deployments: []*defangv1.Deployment{
			{
				Stack:     stackName,
				Provider:  providerEnum,
				Region:    region,
				Timestamp: timestamppb.New(deployedAt),
			},
		},
	}
}

type TestCase struct {
	name                   string
	inputStackName         string
	useWkDir               bool
	localStackExists       bool
	remoteStackExists      bool
	hasOtherLocalStacks    bool
	hasOtherRemoteStacks   bool
	userSelectsCreateNew   bool
	userProviderChoice     string
	userRegionChoice       string
	userStackNameChoice    string
	userStackSelection     string
	expectedStackName      string
	expectedProvider       cliClient.ProviderID
	expectedRegion         string
	expectStackFileWritten bool
	expectNewStackCreated  bool
}

func TestSetupProvider(t *testing.T) {
	testCases := []TestCase{
		{
			name:                   "stackname provided, stackfile exists locally, no previous deployments, useWkDir true, expect stack to be loaded",
			inputStackName:         "teststack",
			useWkDir:               true,
			localStackExists:       true,
			remoteStackExists:      false,
			expectedStackName:      "teststack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  false,
		},
		{
			name:                   "stackname provided, stackfile exists locally, no previous deployments, useWkDir false, expect stack to be loaded",
			inputStackName:         "teststack",
			useWkDir:               false,
			localStackExists:       true,
			remoteStackExists:      false,
			expectedStackName:      "teststack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  false,
		},
		{
			name:                   "stackname provided, stackfile doesn't exist locally, exists in previous deployments, useWkDir true, expect stackfile written and stack loaded",
			inputStackName:         "remotestack",
			useWkDir:               true,
			localStackExists:       false,
			remoteStackExists:      true,
			expectedStackName:      "remotestack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: true,
			expectNewStackCreated:  false,
		},
		{
			name:                   "stackname provided, stackfile doesn't exist locally, exists in previous deployments, useWkDir false, expect remote stack loaded but stackfile not written",
			inputStackName:         "remotestack",
			useWkDir:               false,
			localStackExists:       false,
			remoteStackExists:      true,
			expectedStackName:      "remotestack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  false,
		},
		{
			name:                   "no stackname, no local stackfiles, no deployments, useWkDir true, create stack with stackfile",
			inputStackName:         "",
			useWkDir:               true,
			localStackExists:       false,
			remoteStackExists:      false,
			hasOtherLocalStacks:    false,
			hasOtherRemoteStacks:   false,
			userProviderChoice:     "Defang Playground",
			userStackNameChoice:    "newstack",
			expectedStackName:      "newstack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false, // createNewStack doesn't write the file during tests
			expectNewStackCreated:  true,
		},
		{
			name:                   "no stackname, no local stackfiles, no deployments, useWkDir false, create stack without stackfile",
			inputStackName:         "",
			useWkDir:               false,
			localStackExists:       false,
			remoteStackExists:      false,
			hasOtherLocalStacks:    false,
			hasOtherRemoteStacks:   false,
			userProviderChoice:     "Defang Playground",
			userStackNameChoice:    "newstack",
			expectedStackName:      "newstack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  true,
		},
		{
			name:                   "no stackname, local stackfile exists, useWkDir true, user selects existing stackfile",
			inputStackName:         "",
			useWkDir:               true,
			localStackExists:       false,
			remoteStackExists:      false,
			hasOtherLocalStacks:    true,
			hasOtherRemoteStacks:   false,
			userStackSelection:     "existingstack",
			expectedStackName:      "existingstack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  false,
		},
		{
			name:                   "no stackname, local stackfile exists, useWkDir true, user creates new stack",
			inputStackName:         "",
			useWkDir:               true,
			localStackExists:       false,
			remoteStackExists:      false,
			hasOtherLocalStacks:    true,
			hasOtherRemoteStacks:   false,
			userSelectsCreateNew:   true,
			userProviderChoice:     "Defang Playground",
			userStackNameChoice:    "brandnew",
			expectedStackName:      "brandnew",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  true,
		},
		{
			name:                   "no stackname, local stackfile exists, useWkDir false, create new stack without stackfile",
			inputStackName:         "",
			useWkDir:               false,
			localStackExists:       false,
			remoteStackExists:      false,
			hasOtherLocalStacks:    true,
			hasOtherRemoteStacks:   false,
			userSelectsCreateNew:   true,
			userProviderChoice:     "Defang Playground",
			userStackNameChoice:    "newdefang",
			expectedStackName:      "newdefang",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  true,
		},
		{
			name:                   "no stackname, previous deployment exists, useWkDir true, user selects existing deployment",
			inputStackName:         "",
			useWkDir:               true,
			localStackExists:       false,
			remoteStackExists:      false,
			hasOtherLocalStacks:    false,
			hasOtherRemoteStacks:   true,
			userStackSelection:     "remotestack (last deployed TIME)",
			expectedStackName:      "remotestack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: true,
			expectNewStackCreated:  false,
		},
		{
			name:                   "no stackname, previous deployment exists, useWkDir false, user selects existing deployment",
			inputStackName:         "",
			useWkDir:               false,
			localStackExists:       false,
			remoteStackExists:      false,
			hasOtherLocalStacks:    false,
			hasOtherRemoteStacks:   true,
			userStackSelection:     "remotestack (last deployed TIME)",
			expectedStackName:      "remotestack",
			expectedProvider:       cliClient.ProviderDefang,
			expectedRegion:         "",
			expectStackFileWritten: false,
			expectNewStackCreated:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			projectName := "test-project"
			stackName := tc.inputStackName
			deployTime := time.Now().Add(-24 * time.Hour)

			// Create fresh mocks for each test
			mockPC := &mockProviderCreator{}
			mockEC := &mockElicitationsController{}
			mockFC := &mockFabricClient{}
			mockSM := &mockStacksManager{}
			mockProv := &mockProvider{}

			pp := &providerPreparer{
				pc: mockPC,
				ec: mockEC,
				fc: mockFC,
				sm: mockSM,
			}

			// Set up mocks based on test case
			setupMocks(t, tc, ctx, projectName, deployTime, mockPC, mockEC, mockFC, mockSM, mockProv)

			// Call the function under test - SetupProvider
			providerID, provider, err := pp.SetupProvider(ctx, projectName, &stackName, tc.useWkDir)

			// Assertions
			require.NoError(t, err)
			require.NotNil(t, providerID)
			require.Equal(t, tc.expectedProvider, *providerID)
			require.Equal(t, mockProv, provider)
			require.Equal(t, tc.expectedStackName, stackName)

			// Verify mocks were called as expected
			mockSM.AssertExpectations(t)
			mockFC.AssertExpectations(t)
			mockPC.AssertExpectations(t)
			if tc.userProviderChoice != "" || tc.userStackSelection != "" || tc.userStackNameChoice != "" {
				mockEC.AssertExpectations(t)
			}
		})
	}
}

func setupMocks(t *testing.T, tc TestCase, ctx context.Context, projectName string, deployTime time.Time, mockPC *mockProviderCreator, mockEC *mockElicitationsController, mockFC *mockFabricClient, mockSM *mockStacksManager, mockProv *mockProvider) {
	t.Helper()

	// Handle stackname provided scenarios
	if tc.inputStackName != "" {
		expectedStack := createTestStackParameters(tc.inputStackName, tc.expectedProvider, tc.expectedRegion)

		if tc.localStackExists {
			mockSM.On("Read", tc.inputStackName).Return(expectedStack, nil)
		} else if tc.remoteStackExists {
			mockSM.On("Read", tc.inputStackName).Return((*stacks.StackParameters)(nil), os.ErrNotExist)
			mockFC.On("ListDeployments", ctx, &defangv1.ListDeploymentsRequest{
				Project: projectName,
			}).Return(createDeploymentResponse(tc.inputStackName, "defang", tc.expectedRegion, deployTime), nil)

			if tc.expectStackFileWritten {
				mockSM.On("Create", *expectedStack).Return(".defang/"+tc.inputStackName, nil)
			}
		}

		mockSM.On("Load", tc.inputStackName).Return(nil)
		mockPC.On("NewProvider", ctx, tc.expectedProvider, mockFC, tc.inputStackName).Return(mockProv)
		return
	}

	// Handle no stackname scenarios
	deployments := []*defangv1.Deployment{}
	if tc.hasOtherRemoteStacks {
		deployments = append(deployments, &defangv1.Deployment{
			Stack:     "remotestack",
			Provider:  defangv1.Provider_DEFANG,
			Region:    tc.expectedRegion,
			Timestamp: timestamppb.New(deployTime),
		})
	}
	mockFC.On("ListDeployments", ctx, &defangv1.ListDeploymentsRequest{
		Project: projectName,
	}).Return(&defangv1.ListDeploymentsResponse{Deployments: deployments}, nil)

	// Mock List for local stacks (only called when useWkDir=true)
	if tc.useWkDir {
		localStacks := []stacks.StackListItem{}
		if tc.hasOtherLocalStacks {
			localStacks = append(localStacks, stacks.StackListItem{
				Name:     "existingstack",
				Provider: tc.expectedProvider.String(),
				Region:   tc.expectedRegion,
			})
		}
		mockSM.On("List").Return(localStacks, nil)
	}

	// Mock stack selection or creation
	// Local stacks only matter when useWkDir is true
	if (tc.hasOtherLocalStacks && tc.useWkDir) || tc.hasOtherRemoteStacks {
		stackOptions := []string{}
		if tc.hasOtherLocalStacks && tc.useWkDir {
			stackOptions = append(stackOptions, "existingstack")
		}
		if tc.hasOtherRemoteStacks {
			label := "remotestack (last deployed " + deployTime.Local().Format(time.RFC822) + ")"
			stackOptions = append(stackOptions, label)
			// Update the expected selection to match the actual format
			if strings.Contains(tc.userStackSelection, "remotestack") {
				tc.userStackSelection = label
			}
		}
		if tc.useWkDir {
			stackOptions = append(stackOptions, CreateNewStack)
		}

		selectedOption := tc.userStackSelection
		if tc.userSelectsCreateNew {
			selectedOption = CreateNewStack
		}
		mockEC.On("RequestEnum", ctx, "Select a stack", "stack", stackOptions).Return(selectedOption, nil)

		if tc.userSelectsCreateNew {
			setupNewStackCreationMocks(tc, ctx, mockEC, mockSM)
		} else if tc.userStackSelection == "existingstack" {
			setupExistingLocalStackMocks("existingstack", tc.expectedProvider, tc.expectedRegion, mockSM)
		} else if strings.Contains(tc.userStackSelection, "remotestack") {
			setupExistingRemoteStackMocks("remotestack", tc.expectedProvider, tc.expectedRegion, tc.expectStackFileWritten, mockSM)
		}
	} else if shouldCreateNewStack(tc) {
		// No stack selection needed, directly create new stack
		setupNewStackCreationMocks(tc, ctx, mockEC, mockSM)
	}

	// For new stack creation scenarios, need to mock the final loadStack calls
	expectedStack := createTestStackParameters(tc.expectedStackName, tc.expectedProvider, tc.expectedRegion)
	if tc.expectNewStackCreated {
		mockSM.On("Read", tc.expectedStackName).Return(expectedStack, nil)
		mockSM.On("Load", tc.expectedStackName).Return(nil)
	}

	mockPC.On("NewProvider", ctx, tc.expectedProvider, mockFC, tc.expectedStackName).Return(mockProv)
}

func setupExistingLocalStackMocks(stackName string, expectedProvider cliClient.ProviderID, expectedRegion string, mockSM *mockStacksManager) {
	expectedStack := createTestStackParameters(stackName, expectedProvider, expectedRegion)
	mockSM.On("Read", stackName).Return(expectedStack, nil)
	mockSM.On("Load", stackName).Return(nil)
}

func setupExistingRemoteStackMocks(stackName string, expectedProvider cliClient.ProviderID, expectedRegion string, expectStackFileWritten bool, mockSM *mockStacksManager) {
	expectedStack := createTestStackParameters(stackName, expectedProvider, expectedRegion)
	if expectStackFileWritten {
		mockSM.On("Create", *expectedStack).Return(".defang/"+stackName, nil)
	}
	mockSM.On("Read", stackName).Return(expectedStack, nil)
	mockSM.On("Load", stackName).Return(nil)
}

func shouldCreateNewStack(tc TestCase) bool {
	// User explicitly wants to create new stack
	if tc.userSelectsCreateNew {
		return true
	}

	// When useWkDir=false and there are existing local stacks, still create new
	if !tc.useWkDir && tc.hasOtherLocalStacks {
		return true
	}

	// No existing stacks anywhere, so must create new
	if !tc.hasOtherLocalStacks && !tc.hasOtherRemoteStacks {
		return true
	}

	return false
}

func setupNewStackCreationMocks(tc TestCase, ctx context.Context, mockEC *mockElicitationsController, mockSM *mockStacksManager) {
	providerNames := []string{"Defang Playground", "AWS", "DigitalOcean", "Google Cloud Platform"}
	mockEC.On("RequestEnum", ctx, "Where do you want to deploy?", "provider", providerNames).Return(tc.userProviderChoice, nil)

	if tc.userProviderChoice != "Defang Playground" {
		var defaultRegion string
		switch tc.userProviderChoice {
		case "AWS":
			defaultRegion = "us-east-1"
		case "Google Cloud Platform":
			defaultRegion = "us-central1"
		case "DigitalOcean":
			defaultRegion = "nyc1"
		}
		mockEC.On("RequestStringWithDefault", ctx, "Which region do you want to deploy to?", "region", defaultRegion).Return(tc.userRegionChoice, nil)
	}

	var defaultName string
	switch tc.userProviderChoice {
	case "Defang Playground":
		defaultName = "defang"
	default:
		// For other providers, would need region processing
		defaultName = "defang"
	}

	mockEC.On("RequestStringWithDefault", ctx, "Enter a name for your stack:", "stack_name", defaultName).Return(tc.userStackNameChoice, nil)

	// Mock Create for when useWkDir is true (this will be called by createNewStack)
	if tc.useWkDir {
		expectedParams := stacks.StackParameters{
			Provider: tc.expectedProvider,
			Region:   tc.expectedRegion,
			Name:     tc.userStackNameChoice,
		}
		mockSM.On("Create", expectedParams).Return(".defang/"+tc.userStackNameChoice, nil)
	}
}
