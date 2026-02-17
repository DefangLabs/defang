package stacks

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockElicitationsController mocks the elicitations.Controller interface
type MockElicitationsController struct {
	mock.Mock
}

func (m *MockElicitationsController) RequestString(ctx context.Context, message, field string, opts ...func(*elicitations.Options)) (string, error) {
	args := m.Called(ctx, message, field, opts)
	return args.String(0), args.Error(1)
}

func (m *MockElicitationsController) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	args := m.Called(ctx, message, field, options)
	return args.String(0), args.Error(1)
}

func (m *MockElicitationsController) SetSupported(supported bool) {
	m.Called(supported)
}

func (m *MockElicitationsController) IsSupported() bool {
	args := m.Called()
	return args.Bool(0)
}

type MockStacksManager struct {
	mock.Mock
}

func (m *MockStacksManager) List(ctx context.Context) ([]ListItem, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	result, ok := args.Get(0).([]ListItem)
	if !ok {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *MockStacksManager) Create(params Parameters) (string, error) {
	args := m.Called(params)
	return args.String(0), args.Error(1)
}

type MockAWSProfileLister struct {
	mock.Mock
}

func (m *MockAWSProfileLister) ListProfiles() ([]string, error) {
	args := m.Called()
	profiles, ok := args.Get(0).([]string)
	if !ok {
		return nil, args.Error(1)
	}
	return profiles, args.Error(1)
}

func TestStackSelector_SelectStack_ExistingStack(t *testing.T) {
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []ListItem{
		{
			Parameters: Parameters{
				Name:     "production",
				Provider: "aws",
				Region:   "us-west-2",
			},
		},
		{
			Parameters: Parameters{
				Name:     "development",
				Provider: "aws",
				Region:   "us-east-1",
			},
		},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting existing stack
	expectedOptions := []string{"production (us-west-2)", "development (us-east-1)"}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return("production (us-west-2)", nil)

	// Expected params based on ToParameters() conversion
	expectedParams := &Parameters{
		Name:     "production",
		Provider: client.ProviderAWS,
		Region:   "us-west-2",
		Mode:     modes.ModeUnspecified,
	}

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx, SelectStackOptions{})

	assert.NoError(t, err)
	assert.Equal(t, expectedParams, result)

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectOrCreateStack_ExistingStack(t *testing.T) {
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []ListItem{
		{Parameters: Parameters{Name: "production", Provider: "aws", Region: "us-west-2"}},
		{Parameters: Parameters{Name: "development", Provider: "aws", Region: "us-east-1"}},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting existing stack
	expectedOptions := []string{"production (us-west-2)", "development (us-east-1)", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return("production (us-west-2)", nil)

	// Expected params based on ToParameters() conversion
	expectedParams := &Parameters{
		Name:     "production",
		Provider: client.ProviderAWS,
		Region:   "us-west-2",
		Mode:     modes.ModeUnspecified,
	}

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx, SelectStackOptions{AllowStackCreation: true})

	assert.NoError(t, err)
	assert.Equal(t, expectedParams, result)

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectStack_CreateNewStack(t *testing.T) {
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AWS_REGION")
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []ListItem{
		{Parameters: Parameters{Name: "production", Provider: "aws", Region: "us-west-2"}},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting to create new stack
	expectedOptions := []string{"production (aws, us-west-2)", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return(CreateNewStack, nil)

	// Mock wizard parameter collection - provider selection
	providerOptions := []string{"Defang Playground", "AWS", "DigitalOcean", "Google Cloud Platform"}
	mockEC.On("RequestEnum", ctx, "Where do you want to deploy?", "provider", providerOptions).Return("AWS", nil)

	// Mock wizard parameter collection - region selection (default is us-west-2 for AWS)
	mockEC.On("RequestString", ctx, "Which region do you want to deploy to?", "region", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "us-west-2"
	})).Return("us-east-1", nil)

	// Mock wizard parameter collection - stack name (default name based on provider and region)
	mockEC.On("RequestString", ctx, "What do you want to call this stack?:", "stack_name", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "awsuseast1" && o.Validator != nil
	})).Return("staging", nil)

	// Mock wizard parameter collection - AWS profile selection (both scenarios)
	// If profiles are found on filesystem, it will use RequestEnum
	awsProfileOptions := []string{"default"}
	mockEC.On("RequestEnum", ctx, "Which AWS profile do you want to use?", "aws_profile", awsProfileOptions).Return("staging", nil).Maybe()
	// If no profiles are found, it will use RequestString
	mockEC.On("RequestString", ctx, "Which AWS profile do you want to use?", "aws_profile", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "default"
	})).Return("staging", nil).Maybe()

	// Mock wizard parameter collection
	newStackParams := &Parameters{
		Name:     "staging",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Variables: map[string]string{
			"AWS_PROFILE": "staging",
		},
	}

	// Mock stack creation
	mockSM.On("Create", *newStackParams).Return("staging", nil)

	mockProfileLister := &MockAWSProfileLister{}
	mockProfileLister.On("ListProfiles").Return([]string{"default"}, nil)

	selector := NewSelector(mockEC, mockSM)
	selector.wizard = NewWizardWithProfileLister(mockEC, mockProfileLister)

	result, err := selector.SelectStack(ctx, SelectStackOptions{AllowStackCreation: true})

	assert.NoError(t, err)
	assert.Equal(t, newStackParams, result)

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
	mockProfileLister.AssertExpectations(t)
}

func TestStackSelector_SelectStack_NoExistingStacks(t *testing.T) {
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock empty stacks list - when no stacks exist, it should automatically proceed to create new
	mockSM.On("List", ctx).Return([]ListItem{}, nil)

	// Mock wizard parameter collection - provider selection
	providerOptions := []string{"Defang Playground", "AWS", "DigitalOcean", "Google Cloud Platform"}
	mockEC.On("RequestEnum", ctx, "Where do you want to deploy?", "provider", providerOptions).Return("AWS", nil)

	// Mock wizard parameter collection - region selection
	mockEC.On("RequestString", ctx, "Which region do you want to deploy to?", "region", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "us-west-2"
	})).Return("us-west-2", nil)

	// Mock wizard parameter collection - stack name (default name based on provider and region)
	mockEC.On("RequestString", ctx, "What do you want to call this stack?:", "stack_name", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "awsuswest2" && o.Validator != nil
	})).Return("firststack", nil)

	// Mock wizard parameter collection - AWS profile selection (both scenarios)
	// If profiles are found on filesystem, it will use RequestEnum
	awsProfileOptions := []string{"default"}
	mockEC.On("RequestEnum", ctx, "Which AWS profile do you want to use?", "aws_profile", awsProfileOptions).Return("default", nil).Maybe()
	// If no profiles are found, it will use RequestString
	mockEC.On("RequestString", ctx, "Which AWS profile do you want to use?", "aws_profile", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "default"
	})).Return("default", nil).Maybe()

	// Mock wizard parameter collection
	newStackParams := &Parameters{
		Name:     "firststack",
		Provider: client.ProviderAWS,
		Region:   "us-west-2",
		Variables: map[string]string{
			"AWS_PROFILE": "default",
		},
	}

	// Mock stack creation
	mockSM.On("Create", *newStackParams).Return("firststack", nil)

	mockProfileLister := &MockAWSProfileLister{}
	mockProfileLister.On("ListProfiles").Return([]string{"default"}, nil)

	selector := NewSelector(mockEC, mockSM)
	selector.wizard = NewWizardWithProfileLister(mockEC, mockProfileLister)

	result, err := selector.SelectStack(ctx, SelectStackOptions{AllowStackCreation: true})

	assert.NoError(t, err)
	assert.Equal(t, newStackParams, result)

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
	mockProfileLister.AssertExpectations(t)
}

func TestStackSelector_SelectStack_ElicitationsNotSupported(t *testing.T) {
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are not supported
	mockEC.On("IsSupported").Return(false)

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx, SelectStackOptions{})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "your MCP client does not support elicitations")

	mockEC.AssertExpectations(t)
	mockSM.AssertNotCalled(t, "List")
}

func TestStackSelector_SelectStack_ListStacksError(t *testing.T) {
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock error when listing stacks
	mockSM.On("List", ctx).Return([]ListItem{}, errors.New("failed to access stack storage"))

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx, SelectStackOptions{})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to list stacks")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectStack_ElicitationError(t *testing.T) {
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []ListItem{
		{Parameters: Parameters{Name: "production", Provider: "aws", Region: "us-west-2"}},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock error during elicitation
	expectedOptions := []string{"production (aws, us-west-2)"}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return("", errors.New("user cancelled selection"))

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx, SelectStackOptions{})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to elicit stack choice")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectStack_WizardError(t *testing.T) {
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []ListItem{
		{Parameters: Parameters{Name: "production", Provider: "aws", Region: "us-west-2"}},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting to create new stack
	expectedOptions := []string{"production (aws, us-west-2)", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return(CreateNewStack, nil)

	// Mock wizard parameter collection - provider selection fails
	providerOptions := []string{"Defang Playground", "AWS", "DigitalOcean", "Google Cloud Platform"}
	mockEC.On("RequestEnum", ctx, "Where do you want to deploy?", "provider", providerOptions).Return("", errors.New("user cancelled wizard"))

	selector := NewSelector(mockEC, mockSM)
	result, err := selector.SelectStack(ctx, SelectStackOptions{AllowStackCreation: true})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to collect stack parameters")
	assert.Contains(t, err.Error(), "user cancelled wizard")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectStack_CreateStackError(t *testing.T) {
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AWS_REGION")
	ctx := t.Context()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []ListItem{
		{Parameters: Parameters{Name: "production", Provider: "aws", Region: "us-west-2"}},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting to create new stack
	expectedOptions := []string{"production (aws, us-west-2)", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return(CreateNewStack, nil)

	// Mock wizard parameter collection - provider selection
	providerOptions := []string{"Defang Playground", "AWS", "DigitalOcean", "Google Cloud Platform"}
	mockEC.On("RequestEnum", ctx, "Where do you want to deploy?", "provider", providerOptions).Return("AWS", nil)

	// Mock wizard parameter collection - region selection
	mockEC.On("RequestString", ctx, "Which region do you want to deploy to?", "region", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "us-west-2"
	})).Return("us-east-1", nil)

	// Mock wizard parameter collection - stack name (default name based on provider and region)
	mockEC.On("RequestString", ctx, "What do you want to call this stack?:", "stack_name", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "awsuseast1" && o.Validator != nil
	})).Return("staging", nil)

	// Mock wizard parameter collection - AWS profile selection (both scenarios)
	// If profiles are found on filesystem, it will use RequestEnum
	awsProfileOptions := []string{"default"}
	mockEC.On("RequestEnum", ctx, "Which AWS profile do you want to use?", "aws_profile", awsProfileOptions).Return("staging", nil).Maybe()
	// If no profiles are found, it will use RequestString
	mockEC.On("RequestString", ctx, "Which AWS profile do you want to use?", "aws_profile", mock.MatchedBy(func(opts []func(*elicitations.Options)) bool {
		o := &elicitations.Options{}
		for _, opt := range opts {
			opt(o)
		}
		return o.DefaultValue == "default"
	})).Return("staging", nil).Maybe()

	// Mock wizard parameter collection
	newStackParams := &Parameters{
		Name:     "staging",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Variables: map[string]string{
			"AWS_PROFILE": "staging",
		},
	}

	// Mock stack creation error
	mockSM.On("Create", *newStackParams).Return("", errors.New("invalid stack configuration"))

	mockProfileLister := &MockAWSProfileLister{}
	mockProfileLister.On("ListProfiles").Return([]string{"default"}, nil)

	selector := NewSelector(mockEC, mockSM)
	selector.wizard = NewWizardWithProfileLister(mockEC, mockProfileLister)
	result, err := selector.SelectStack(ctx, SelectStackOptions{AllowStackCreation: true})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create stack")
	assert.Contains(t, err.Error(), "invalid stack configuration")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
	mockProfileLister.AssertExpectations(t)
}

func TestMakeStackSelectorLabels(t *testing.T) {
	tests := []struct {
		name       string
		stacks     []ListItem
		wantLabels []string
	}{
		{
			name:       "no stacks",
			stacks:     []ListItem{},
			wantLabels: []string{},
		},
		{
			name: "one stack - present all fields",
			stacks: []ListItem{
				{Parameters: Parameters{Name: "production", Provider: "aws", Region: "us-west-2"}},
			},
			wantLabels: []string{"production (aws, us-west-2)"},
		},
		{
			name: "hide redundant provider",
			stacks: []ListItem{
				{Parameters: Parameters{Name: "production", Provider: "aws", Region: "us-west-2"}},
				{Parameters: Parameters{Name: "development", Provider: "aws", Region: "us-east-1"}},
			},
			wantLabels: []string{
				"production (us-west-2)",
				"development (us-east-1)",
			},
		},
		{
			name: "hide redundant provider and region",
			stacks: []ListItem{
				{Parameters: Parameters{Name: "prod-us-west-2", Provider: "aws", Region: "us-west-2"}},
				{Parameters: Parameters{Name: "dev-us-west-2", Provider: "aws", Region: "us-west-2"}},
			},
			wantLabels: []string{
				"prod-us-west-2",
				"dev-us-west-2",
			},
		},
		{
			name: "mixed redundancy",
			stacks: []ListItem{
				{Parameters: Parameters{Name: "prod-us-west-2", Provider: "aws", Region: "us-west-2"}},
				{Parameters: Parameters{Name: "dev-us-east-1", Provider: "aws", Region: "us-east-1"}},
				{Parameters: Parameters{Name: "gcp-stack", Provider: "gcp", Region: "us-central1"}},
			},
			wantLabels: []string{
				"prod-us-west-2 (aws, us-west-2)",
				"dev-us-east-1 (aws, us-east-1)",
				"gcp-stack (gcp, us-central1)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := MakeStackSelectorLabels(tt.stacks)

			// Extract labels into a slice for easier comparison
			var gotLabels []string
			for label := range labels {
				gotLabels = append(gotLabels, label)
			}

			assert.ElementsMatch(t, tt.wantLabels, gotLabels)
		})
	}
}
