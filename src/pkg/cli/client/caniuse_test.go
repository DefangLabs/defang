package client

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
)

// mockCanIUseProvider implements Provider with just the methods CanIUseProvider needs.
type mockCanIUseProvider struct {
	Provider      // embed for unused methods
	projectUpdate *defangv1.ProjectUpdate
	projectErr    error
	config        *defangv1.CanIUseResponse // captured by SetCanIUseConfig
}

func (m *mockCanIUseProvider) AccountInfo(context.Context) (*AccountInfo, error) {
	return &AccountInfo{AccountID: "123", Provider: ProviderAWS, Region: "us-east-1"}, nil
}

func (*mockCanIUseProvider) Driver() string {
	return "mock-driver"
}

func (m *mockCanIUseProvider) GetProjectUpdate(_ context.Context, _ string) (*defangv1.ProjectUpdate, error) {
	if m.projectErr != nil {
		return nil, m.projectErr
	}
	if m.projectUpdate == nil {
		return nil, ErrNotExist
	}
	return m.projectUpdate, nil
}

func (m *mockCanIUseProvider) GetStackName() string { return "test" }

func (m *mockCanIUseProvider) SetCanIUseConfig(resp *defangv1.CanIUseResponse) {
	m.config = resp
}

// mockCanIUseFabric implements FabricClient.CanIUse with configurable response.
type mockCanIUseFabric struct {
	FabricClient // embed for unused methods
	resp         *defangv1.CanIUseResponse
	err          error
	lastReq      *defangv1.CanIUseRequest // captured request
}

func (m *mockCanIUseFabric) CanIUse(_ context.Context, req *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error) {
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	// Return a copy so tests can't accidentally share state
	//nolint:forcetypeassert
	return proto.Clone(m.resp).(*defangv1.CanIUseResponse), nil
}

func TestCanIUseProvider(t *testing.T) {
	ctx := t.Context()
	term.SetDebug(testing.Verbose())

	tests := []struct {
		name          string
		projectName   string
		allowUpgrade  bool
		cdEnv         string // DEFANG_CD_IMAGE override
		pulumiEnv     string // DEFANG_PULUMI_VERSION override
		fabricCD      string // what fabric returns
		fabricPulumi  string // what fabric returns
		fabricForced  bool   // force upgrade flag
		fabricErr     error  // CanIUse error (e.g. permission denied)
		prevCD        string // previously deployed
		prevPulumi    string // previously deployed
		prevErr       error  // GetProjectUpdate error
		wantErr       bool   // expect CanIUseProvider to return error
		wantCD        string // expected final CD image
		wantPulumi    string // expected final Pulumi version
		wantReqCD     string // expected CdVersion in request to fabric
		wantReqPulumi string // expected PulumiVersion in request to fabric
	}{
		{
			name:         "new project uses latest from fabric",
			projectName:  "myproject",
			fabricCD:     "cd:v2",
			fabricPulumi: "3.100",
			// prevErr is nil, projectUpdate is nil → providers return (nil, nil) for new projects
			wantCD:     "cd:v2",
			wantPulumi: "3.100",
		},
		{
			name:          "fabric can override even with allowUpgrade false",
			projectName:   "myproject",
			fabricCD:      "cd:v2",
			fabricPulumi:  "3.100",
			fabricForced:  true,
			prevCD:        "cd:v1",
			prevPulumi:    "3.90",
			wantCD:        "cd:v2",
			wantPulumi:    "3.100",
			wantReqCD:     "cd:v1",
			wantReqPulumi: "3.90",
		},
		{
			name:          "allow-upgrade uses latest from fabric",
			projectName:   "myproject",
			allowUpgrade:  true,
			fabricCD:      "cd:v2",
			fabricPulumi:  "3.100",
			prevCD:        "cd:v1",
			prevPulumi:    "3.90",
			wantCD:        "cd:v2",
			wantPulumi:    "3.100",
			wantReqCD:     "", // not sent when allowUpgrade
			wantReqPulumi: "",
		},
		{
			name:         "env override takes precedence over everything",
			projectName:  "myproject",
			cdEnv:        "cd:custom",
			pulumiEnv:    "3.50",
			fabricCD:     "cd:v2",
			fabricPulumi: "3.100",
			prevCD:       "cd:v1",
			prevPulumi:   "3.90",
			wantCD:       "cd:custom",
			wantPulumi:   "3.50",
		},
		{
			name:          "same versions no change",
			projectName:   "myproject",
			fabricCD:      "cd:v1",
			fabricPulumi:  "3.90",
			prevCD:        "cd:v1",
			prevPulumi:    "3.90",
			wantCD:        "cd:v1",
			wantPulumi:    "3.90",
			wantReqCD:     "cd:v1",
			wantReqPulumi: "3.90",
		},
		{
			name:         "empty project name skips pinning",
			projectName:  "",
			fabricCD:     "cd:v2",
			fabricPulumi: "3.100",
			wantCD:       "cd:v2",
			wantPulumi:   "3.100",
		},
		{
			name:         "nil project update uses latest",
			projectName:  "myproject",
			fabricCD:     "cd:v2",
			fabricPulumi: "3.100",
			// prevCD/prevPulumi left empty, projectUpdate will be nil
			wantCD:     "cd:v2",
			wantPulumi: "3.100",
		},
		{
			name:          "previous CD set but previous Pulumi empty pins CD only",
			projectName:   "myproject",
			fabricCD:      "cd:v2",
			fabricPulumi:  "3.100",
			prevCD:        "cd:v1",
			prevPulumi:    "",
			wantCD:        "cd:v1",
			wantPulumi:    "3.100",
			wantReqCD:     "cd:v1",
			wantReqPulumi: "",
		},
		{
			name:          "partial env override: CD from env, Pulumi pinned",
			projectName:   "myproject",
			cdEnv:         "cd:custom",
			fabricCD:      "cd:v2",
			fabricPulumi:  "3.100",
			prevCD:        "cd:v1",
			prevPulumi:    "3.90",
			wantCD:        "cd:custom",
			wantPulumi:    "3.90",
			wantReqCD:     "cd:v1", // still fetched because Pulumi needs pinning
			wantReqPulumi: "3.90",
		},
		{
			name:         "GetProjectUpdate error propagates",
			projectName:  "myproject",
			fabricCD:     "cd:v2",
			fabricPulumi: "3.100",
			prevErr:      errors.New("transient S3 error"),
			wantErr:      true,
		},
		{
			name:        "fabric error propagates",
			projectName: "myproject",
			fabricErr:   errors.New("permission denied"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cdEnv != "" {
				t.Setenv("DEFANG_CD_IMAGE", tt.cdEnv)
			}
			if tt.pulumiEnv != "" {
				t.Setenv("DEFANG_PULUMI_VERSION", tt.pulumiEnv)
			}

			var projUpdate *defangv1.ProjectUpdate
			if tt.prevCD != "" || tt.prevPulumi != "" {
				projUpdate = &defangv1.ProjectUpdate{
					CdVersion:     tt.prevCD,
					PulumiVersion: tt.prevPulumi,
				}
			}

			provider := &mockCanIUseProvider{
				projectUpdate: projUpdate,
				projectErr:    tt.prevErr,
			}
			fabric := &mockCanIUseFabric{
				resp: &defangv1.CanIUseResponse{
					CdImage:       tt.fabricCD,
					PulumiVersion: tt.fabricPulumi,
					ForcedVersion: tt.fabricForced,
				},
				err: tt.fabricErr,
			}

			err := CanIUseProvider(ctx, fabric, provider, tt.projectName, 1, tt.allowUpgrade)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CanIUseProvider() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("CanIUseProvider() error: %v", err)
			}

			if provider.config.CdImage != tt.wantCD {
				t.Errorf("CdImage = %q, want %q", provider.config.CdImage, tt.wantCD)
			}
			if provider.config.PulumiVersion != tt.wantPulumi {
				t.Errorf("PulumiVersion = %q, want %q", provider.config.PulumiVersion, tt.wantPulumi)
			}

			// Verify what was sent to the fabric
			if fabric.lastReq.PreferCdVersion != tt.wantReqCD {
				t.Errorf("request CdVersion = %q, want %q", fabric.lastReq.PreferCdVersion, tt.wantReqCD)
			}
			if fabric.lastReq.PreferPulumiVersion != tt.wantReqPulumi {
				t.Errorf("request PulumiVersion = %q, want %q", fabric.lastReq.PreferPulumiVersion, tt.wantReqPulumi)
			}
		})
	}
}

func TestPinVersion(t *testing.T) {
	tests := []struct {
		name     string
		latest   string
		previous string
		label    string
		upgrade  bool
		want     string
	}{
		{
			name:     "empty previous returns latest",
			latest:   "v2",
			previous: "",
			upgrade:  false,
			want:     "v2",
		},
		{
			name:     "same version returns latest",
			latest:   "v1",
			previous: "v1",
			upgrade:  false,
			want:     "v1",
		},
		{
			name:     "newer available without upgrade returns previous",
			latest:   "v2",
			previous: "v1",
			upgrade:  false,
			want:     "v1",
		},
		{
			name:     "newer available with upgrade returns latest",
			latest:   "v2",
			previous: "v1",
			upgrade:  true,
			want:     "v2",
		},
		{
			name:     "downgrade without upgrade returns previous",
			latest:   "v1",
			previous: "v2",
			upgrade:  false,
			want:     "v2",
		},
		{
			name:     "downgrade with upgrade returns latest",
			latest:   "v1",
			previous: "v2",
			upgrade:  true,
			want:     "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveVersion("", tt.latest, tt.previous, "test", tt.upgrade, false)
			if got != tt.want {
				t.Errorf("resolveVersion(%q, %q, upgrade=%v) = %q, want %q", tt.latest, tt.previous, tt.upgrade, got, tt.want)
			}
		})
	}
}
