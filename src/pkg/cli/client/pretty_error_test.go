package client

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
)

func TestPrettyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "bare connect error strips the gRPC code",
			err:  connect.NewError(connect.CodeInvalidArgument, errors.New("this recipe is not available for your tier. See https://s.defang.io/subscription")),
			want: "this recipe is not available for your tier. See https://s.defang.io/subscription",
		},
		{
			name: "wrapped connect error keeps outer context",
			err: fmt.Errorf("failed to get recipe for deployment mode %q: %w", "primary",
				connect.NewError(connect.CodeInvalidArgument, errors.New("this recipe is not available for your tier. See https://s.defang.io/subscription"))),
			want: `failed to get recipe for deployment mode "primary": this recipe is not available for your tier. See https://s.defang.io/subscription`,
		},
		{
			name: "non-connect error is returned unchanged",
			err:  errors.New("some local error"),
			want: "some local error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrettyError(tt.err)
			if got.Error() != tt.want {
				t.Errorf("PrettyError() = %q, want %q", got.Error(), tt.want)
			}
		})
	}
}

func TestPrettyError_PreservesSentinelForErrorsIs(t *testing.T) {
	sentinel := errors.New("rate limited")
	cerr := connect.NewError(connect.CodeResourceExhausted, sentinel)
	wrapped := fmt.Errorf("deploying project %q: %w", "myproject", cerr)

	got := PrettyError(wrapped)

	if !errors.Is(got, sentinel) {
		t.Errorf("errors.Is(PrettyError(err), sentinel) = false, want true")
	}
	want := `deploying project "myproject": rate limited`
	if got.Error() != want {
		t.Errorf("PrettyError() = %q, want %q", got.Error(), want)
	}
}
