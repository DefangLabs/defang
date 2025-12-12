package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/smithy-go/ptr"
)

type mockSM struct {
	UpdateFn  func(ctx context.Context, params *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error)
	CreateFn  func(ctx context.Context, params *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error)
	RestoreFn func(ctx context.Context, params *secretsmanager.RestoreSecretInput) (*secretsmanager.RestoreSecretOutput, error)
}

func (m *mockSM) UpdateSecret(ctx context.Context, in *secretsmanager.UpdateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
	return m.UpdateFn(ctx, in)
}
func (m *mockSM) CreateSecret(ctx context.Context, in *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	return m.CreateFn(ctx, in)
}
func (m *mockSM) RestoreSecret(ctx context.Context, in *secretsmanager.RestoreSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error) {
	return m.RestoreFn(ctx, in)
}

func TestPutSecretManagerSecret(t *testing.T) {
	tests := []struct {
		name        string
		mock        func() *mockSM
		wantErr     bool
		wantVersion string
	}{
		{
			name: "update succeeds directly",
			mock: func() *mockSM {
				return &mockSM{
					UpdateFn: func(ctx context.Context, p *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
						return &secretsmanager.UpdateSecretOutput{
							ARN:       ptr.String("arn:aws:secret:123"),
							VersionId: ptr.String("v1"),
						}, nil
					},
					CreateFn: nil,
				}
			},
			wantErr:     false,
			wantVersion: "arn:aws:secret:123::v1",
		},
		{
			name: "update returns NotFound, create succeeds, second update succeeds",
			mock: func() *mockSM {
				calledCreate := false

				return &mockSM{
					UpdateFn: func(ctx context.Context, p *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
						if !calledCreate {
							// First call - trigger ResourceNotFound
							return nil, &types.ResourceNotFoundException{}
						}
						// Second call - success
						return &secretsmanager.UpdateSecretOutput{
							ARN:       ptr.String("arn:aws:secret:new"),
							VersionId: ptr.String("v2"),
						}, nil
					},
					CreateFn: func(ctx context.Context, p *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
						calledCreate = true
						return &secretsmanager.CreateSecretOutput{}, nil
					},
				}
			},
			wantErr:     false,
			wantVersion: "arn:aws:secret:new::v2",
		},
		{
			name: "update returns secret marked for deletion, restore succeeds, second update succeeds",
			mock: func() *mockSM {
				calledCreate, calledRestore := false, false

				return &mockSM{
					UpdateFn: func(ctx context.Context, p *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
						if !calledRestore {
							// First call - simulate secret was marked for deletion
							return nil, &types.InvalidRequestException{
								Message: ptr.String("You can't perform this operation on the secret because it was marked for deletion."),
							}
						}
						if calledCreate {
							return nil, errors.New("should not call create when secret is marked for deletion")
						}
						// Second call - success
						return &secretsmanager.UpdateSecretOutput{
							ARN:       ptr.String("arn:aws:secret:new"),
							VersionId: ptr.String("v2"),
						}, nil
					},
					CreateFn: func(ctx context.Context, p *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
						calledCreate = true
						return &secretsmanager.CreateSecretOutput{}, nil
					},
					RestoreFn: func(ctx context.Context, p *secretsmanager.RestoreSecretInput) (*secretsmanager.RestoreSecretOutput, error) {
						calledRestore = true
						return &secretsmanager.RestoreSecretOutput{}, nil
					},
				}
			},
			wantErr:     false,
			wantVersion: "arn:aws:secret:new::v2",
		},
		{
			name: "update fails with non-NotFound error",
			mock: func() *mockSM {
				return &mockSM{
					UpdateFn: func(ctx context.Context, p *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
						return nil, errors.New("other error")
					},
					CreateFn: nil,
				}
			},
			wantErr: true,
		},
		{
			name: "create fails with non-Exists error",
			mock: func() *mockSM {
				return &mockSM{
					UpdateFn: func(ctx context.Context, p *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
						return nil, &types.ResourceNotFoundException{}
					},
					CreateFn: func(ctx context.Context, p *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
						return nil, errors.New("create failed")
					},
				}
			},
			wantErr: true,
		},
		{
			name: "missing ARN or VersionId in final update output",
			mock: func() *mockSM {
				calledCreate := false
				return &mockSM{
					UpdateFn: func(ctx context.Context, p *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
						if !calledCreate {
							return nil, &types.ResourceNotFoundException{}
						}
						return &secretsmanager.UpdateSecretOutput{
							ARN:       nil,
							VersionId: ptr.String("v3"),
						}, nil
					},
					CreateFn: func(ctx context.Context, p *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
						calledCreate = true
						return &secretsmanager.CreateSecretOutput{}, nil
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.mock()
			got, err := PutSecretManagerSecret(t.Context(), "my-secret", "val", svc)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			if got != tt.wantVersion {
				t.Fatalf("expected %q, got %q", tt.wantVersion, got)
			}
		})
	}
}
