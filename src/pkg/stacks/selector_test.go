package stacks

import (
	"context"
	"errors"
	"fmt"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockElicitationsController mocks the elicitations.Controller interface
type MockElicitationsController struct {
	mock.Mock
}

func (m *MockElicitationsController) RequestString(ctx context.Context, message, field string) (string, error) {
	args := m.Called(ctx, message, field)
	return args.String(0), args.Error(1)
}

func (m *MockElicitationsController) RequestStringWithDefault(ctx context.Context, message, field, defaultValue string) (string, error) {
	args := m.Called(ctx, message, field, defaultValue)
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

// MockStacksManager mocks the stacks.Manager interface
type MockStacksManager struct {
	mock.Mock
}

func (m *MockStacksManager) List(ctx context.Context) ([]StackListItem, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	result, ok := args.Get(0).([]StackListItem)
	if !ok {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *MockStacksManager) Load(name string) (*StackParameters, error) {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	result, ok := args.Get(0).(*StackParameters)
	if !ok {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *MockStacksManager) LoadParameters(params map[string]string, overload bool) error {
	args := m.Called(params, overload)
	return args.Error(0)
}

func (m *MockStacksManager) Create(params StackParameters) (string, error) {
	args := m.Called(params)
	return args.String(0), args.Error(1)
}

func TestStackSelector_SelectStack_ExistingStack(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []StackListItem{
		{Name: "production", Provider: "aws", Region: "us-west-2"},
		{Name: "development", Provider: "aws", Region: "us-east-1"},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting existing stack
	expectedOptions := []string{"production", "development", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return("production", nil)

	// Mock loading the selected stack
	expectedParams := &StackParameters{
		Name:       "production",
		Provider:   cliClient.ProviderAWS,
		Region:     "us-west-2",
		AWSProfile: "default",
		Mode:       modes.ModeBalanced,
	}
	mockSM.On("Load", "production").Return(expectedParams, nil)

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx)

	assert.NoError(t, err)
	assert.Equal(t, expectedParams, result)

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

// WizardInterface defines the interface for collecting stack parameters
type WizardInterface interface {
	CollectParameters(ctx context.Context) (*StackParameters, error)
}

// MockWizardInterface mocks the WizardInterface
type MockWizardInterface struct {
	mock.Mock
}

func (m *MockWizardInterface) CollectParameters(ctx context.Context) (*StackParameters, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	result, ok := args.Get(0).(*StackParameters)
	if !ok {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

// testableStackSelector extends stackSelector to allow wizard injection for testing
type testableStackSelector struct {
	ec     elicitations.Controller
	sm     Manager
	wizard WizardInterface
}

func (tss *testableStackSelector) SelectStack(ctx context.Context) (*StackParameters, error) {
	if !tss.ec.IsSupported() {
		return nil, errors.New("your mcp client does not support elicitations, use the 'select_stack' tool to choose a stack")
	}
	selectedStackName, err := tss.elicitStackSelection(ctx, tss.ec)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	if selectedStackName == CreateNewStack {
		params, err := tss.wizard.CollectParameters(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect stack parameters: %w", err)
		}
		_, err = tss.sm.Create(*params)
		if err != nil {
			return nil, fmt.Errorf("failed to create stack: %w", err)
		}

		selectedStackName = params.Name
	}

	return tss.sm.Load(selectedStackName)
}

func (tss *testableStackSelector) elicitStackSelection(ctx context.Context, ec elicitations.Controller) (string, error) {
	stackList, err := tss.sm.List(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list stacks: %w", err)
	}

	if len(stackList) == 0 {
		return CreateNewStack, nil
	}

	stackNames := make([]string, 0, len(stackList)+1)
	for _, s := range stackList {
		stackNames = append(stackNames, s.Name)
	}
	stackNames = append(stackNames, CreateNewStack)

	selectedStackName, err := ec.RequestEnum(ctx, "Select a stack", "stack", stackNames)
	if err != nil {
		return "", fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	return selectedStackName, nil
}

func TestStackSelector_SelectStack_CreateNewStack(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}
	mockWizard := &MockWizardInterface{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []StackListItem{
		{Name: "production", Provider: "aws", Region: "us-west-2"},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting to create new stack
	expectedOptions := []string{"production", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return(CreateNewStack, nil)

	// Mock wizard parameter collection
	newStackParams := &StackParameters{
		Name:       "staging",
		Provider:   cliClient.ProviderAWS,
		Region:     "us-east-1",
		AWSProfile: "staging",
		Mode:       modes.ModeAffordable,
	}
	mockWizard.On("CollectParameters", ctx).Return(newStackParams, nil)

	// Mock stack creation
	mockSM.On("Create", *newStackParams).Return("staging", nil)

	// Mock loading the created stack
	mockSM.On("Load", "staging").Return(newStackParams, nil)

	selector := &testableStackSelector{
		ec:     mockEC,
		sm:     mockSM,
		wizard: mockWizard,
	}

	result, err := selector.SelectStack(ctx)

	assert.NoError(t, err)
	assert.Equal(t, newStackParams, result)

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
	mockWizard.AssertExpectations(t)
}

func TestStackSelector_SelectStack_NoExistingStacks(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}
	mockWizard := &MockWizardInterface{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock empty stacks list - when no stacks exist, it should automatically proceed to create new
	mockSM.On("List", ctx).Return([]StackListItem{}, nil)

	// Mock wizard parameter collection
	newStackParams := &StackParameters{
		Name:       "firststack",
		Provider:   cliClient.ProviderAWS,
		Region:     "us-west-2",
		AWSProfile: "default",
		Mode:       modes.ModeBalanced,
	}
	mockWizard.On("CollectParameters", ctx).Return(newStackParams, nil)

	// Mock stack creation
	mockSM.On("Create", *newStackParams).Return("firststack", nil)

	// Mock loading the created stack
	mockSM.On("Load", "firststack").Return(newStackParams, nil)

	selector := &testableStackSelector{
		ec:     mockEC,
		sm:     mockSM,
		wizard: mockWizard,
	}

	result, err := selector.SelectStack(ctx)

	assert.NoError(t, err)
	assert.Equal(t, newStackParams, result)

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
	mockWizard.AssertExpectations(t)
}

func TestStackSelector_SelectStack_ElicitationsNotSupported(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are not supported
	mockEC.On("IsSupported").Return(false)

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "your mcp client does not support elicitations")

	mockEC.AssertExpectations(t)
	mockSM.AssertNotCalled(t, "List")
}

func TestStackSelector_SelectStack_ListStacksError(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock error when listing stacks
	mockSM.On("List", ctx).Return([]StackListItem{}, errors.New("failed to access stack storage"))

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to select stack")
	assert.Contains(t, err.Error(), "failed to list stacks")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectStack_ElicitationError(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []StackListItem{
		{Name: "production", Provider: "aws", Region: "us-west-2"},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock error during elicitation
	expectedOptions := []string{"production", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return("", errors.New("user cancelled selection"))

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to select stack")
	assert.Contains(t, err.Error(), "failed to elicit stack choice")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectStack_LoadStackError(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []StackListItem{
		{Name: "production", Provider: "aws", Region: "us-west-2"},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting existing stack
	expectedOptions := []string{"production", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return("production", nil)

	// Mock error when loading the selected stack
	mockSM.On("Load", "production").Return((*StackParameters)(nil), errors.New("stack file corrupted"))

	selector := NewSelector(mockEC, mockSM)

	result, err := selector.SelectStack(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "stack file corrupted")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}

func TestStackSelector_SelectStack_WizardError(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}
	mockWizard := &MockWizardInterface{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []StackListItem{
		{Name: "production", Provider: "aws", Region: "us-west-2"},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting to create new stack
	expectedOptions := []string{"production", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return(CreateNewStack, nil)

	// Mock wizard parameter collection error
	mockWizard.On("CollectParameters", ctx).Return((*StackParameters)(nil), errors.New("user cancelled wizard"))

	selector := &testableStackSelector{
		ec:     mockEC,
		sm:     mockSM,
		wizard: mockWizard,
	}

	result, err := selector.SelectStack(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to collect stack parameters")
	assert.Contains(t, err.Error(), "user cancelled wizard")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
	mockWizard.AssertExpectations(t)
}

func TestStackSelector_SelectStack_CreateStackError(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}
	mockWizard := &MockWizardInterface{}

	// Mock that elicitations are supported
	mockEC.On("IsSupported").Return(true)

	// Mock existing stacks list
	existingStacks := []StackListItem{
		{Name: "production", Provider: "aws", Region: "us-west-2"},
	}
	mockSM.On("List", ctx).Return(existingStacks, nil)

	// Mock user selecting to create new stack
	expectedOptions := []string{"production", CreateNewStack}
	mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return(CreateNewStack, nil)

	// Mock wizard parameter collection
	newStackParams := &StackParameters{
		Name:       "staging",
		Provider:   cliClient.ProviderAWS,
		Region:     "us-east-1",
		AWSProfile: "staging",
		Mode:       modes.ModeAffordable,
	}
	mockWizard.On("CollectParameters", ctx).Return(newStackParams, nil)

	// Mock stack creation error
	mockSM.On("Create", *newStackParams).Return("", errors.New("invalid stack configuration"))

	selector := &testableStackSelector{
		ec:     mockEC,
		sm:     mockSM,
		wizard: mockWizard,
	}

	result, err := selector.SelectStack(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create stack")
	assert.Contains(t, err.Error(), "invalid stack configuration")

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
	mockWizard.AssertExpectations(t)
}

func TestStackSelector_ElicitStackSelection(t *testing.T) {
	ctx := context.Background()

	mockEC := &MockElicitationsController{}
	mockSM := &MockStacksManager{}

	// Test case: multiple stacks available
	t.Run("multiple stacks", func(t *testing.T) {
		existingStacks := []StackListItem{
			{Name: "prod", Provider: "aws", Region: "us-west-2"},
			{Name: "dev", Provider: "gcp", Region: "us-central1"},
		}
		mockSM.On("List", ctx).Return(existingStacks, nil).Once()

		expectedOptions := []string{"prod", "dev", CreateNewStack}
		mockEC.On("RequestEnum", ctx, "Select a stack", "stack", expectedOptions).Return("dev", nil).Once()

		selector := NewSelector(mockEC, mockSM)
		result, err := selector.elicitStackSelection(ctx, mockEC)

		assert.NoError(t, err)
		assert.Equal(t, "dev", result)
	})

	// Test case: no stacks available
	t.Run("no stacks", func(t *testing.T) {
		mockSM.On("List", ctx).Return([]StackListItem{}, nil).Once()

		selector := NewSelector(mockEC, mockSM)
		result, err := selector.elicitStackSelection(ctx, mockEC)

		assert.NoError(t, err)
		assert.Equal(t, CreateNewStack, result)
	})

	mockEC.AssertExpectations(t)
	mockSM.AssertExpectations(t)
}
