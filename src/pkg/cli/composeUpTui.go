package cli

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type deploymentModel struct {
	services map[string]*serviceState
	quitting bool
	updateCh chan serviceUpdate
}

type serviceState struct {
	status  defangv1.ServiceState
	spinner spinner.Model
}

type serviceUpdate struct {
	name   string
	status defangv1.ServiceState
}

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#bc9724", Dark: "#2ddedc"})
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#a4729d", Dark: "#fae856"})
	nameStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#305897", Dark: "#cdd2c9"})
)

func newDeploymentModel(serviceNames []string) *deploymentModel {
	services := make(map[string]*serviceState)

	for _, name := range serviceNames {
		s := spinner.New()
		s.Spinner = spinner.Dot
		s.Style = spinnerStyle

		services[name] = &serviceState{
			status:  defangv1.ServiceState_DEPLOYMENT_PENDING,
			spinner: s,
		}
	}

	return &deploymentModel{
		services: services,
		updateCh: make(chan serviceUpdate, 100),
	}
}

func (m *deploymentModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, svc := range m.services {
		cmds = append(cmds, svc.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

func (m *deploymentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case serviceUpdate:
		if svc, exists := m.services[msg.name]; exists {
			svc.status = msg.status
		}
		return m, nil
	case spinner.TickMsg:
		var cmds []tea.Cmd
		for _, svc := range m.services {
			var cmd tea.Cmd
			svc.spinner, cmd = svc.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *deploymentModel) View() string {
	if m.quitting {
		return ""
	}

	var lines []string
	// Sort services by name for consistent ordering
	var serviceNames []string
	for name := range m.services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	for _, name := range serviceNames {
		svc := m.services[name]

		// Stop spinner for completed services
		var spinnerOrCheck string
		switch svc.status {
		case defangv1.ServiceState_DEPLOYMENT_COMPLETED:
			spinnerOrCheck = "✓ "
		case defangv1.ServiceState_DEPLOYMENT_FAILED:
			spinnerOrCheck = "✗ "
		default:
			spinnerOrCheck = svc.spinner.View()
		}

		var statusText string
		switch svc.status {
		case defangv1.ServiceState_NOT_SPECIFIED:
			statusText = ""
		case defangv1.ServiceState_DEPLOYMENT_PENDING:
			statusText = "DEPLOYING"
		default:
			statusText = svc.status.String()
		}

		line := lipgloss.JoinHorizontal(
			lipgloss.Left,
			spinnerOrCheck,
			" ",
			nameStyle.Render("["+name+"]"),
			" ",
			statusStyle.Render(statusText),
		)
		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func MonitorWithUI(ctx context.Context, project *compose.Project, provider client.Provider, waitTimeout time.Duration, deploymentID string) (map[string]defangv1.ServiceState, error) {
	servicesNames := make([]string, 0, len(project.Services))
	for _, svc := range project.Services {
		servicesNames = append(servicesNames, svc.Name)
	}

	// Initialize the bubbletea model
	model := newDeploymentModel(servicesNames)

	// Create the bubbletea program
	p := tea.NewProgram(model)

	var (
		serviceStates map[string]defangv1.ServiceState
		monitorErr    error
		wg            sync.WaitGroup
	)
	wg.Add(2) // One for UI, one for monitoring

	// Start the bubbletea UI in a goroutine
	go func() {
		defer wg.Done()
		if _, err := p.Run(); err != nil {
			// Handle UI errors if needed
		}
	}()

	// Start monitoring in a goroutine
	go func() {
		defer wg.Done()
		serviceStates, monitorErr = Monitor(ctx, project, provider, waitTimeout, deploymentID, func(msg *defangv1.SubscribeResponse, states *ServiceStates) error {
			// Send service status updates to the bubbletea model
			for name, state := range *states {
				p.Send(serviceUpdate{
					name:   name,
					status: state,
				})
			}
			return nil
		})
		// empty out all of the service statuses before printing a final state
		for _, name := range servicesNames {
			p.Send(serviceUpdate{
				name:   name,
				status: defangv1.ServiceState_NOT_SPECIFIED,
			})
		}
		// Quit the UI when monitoring is done
		p.Quit()
	}()

	wg.Wait()

	return serviceStates, monitorErr
}
