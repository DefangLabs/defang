package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

var ErrTermsNotAgreed = errors.New("You must agree to the Defang terms of service to use this tool")

func InteractiveAgreeToS(ctx context.Context, client client.Client) error {
	fmt.Println("Our latest terms of service can be found at https://defang.io/terms-service.html")

	var agreeToS bool
	err := survey.AskOne(&survey.Confirm{
		Message: "Do you agree to the Defang terms of service?",
		Help:    "You must agree to the Defang terms of service to continue using this tool",
	}, &agreeToS)
	if err != nil {
		return err
	}

	if !agreeToS {
		return ErrTermsNotAgreed
	}

	return NonInteractiveAgreeToS(ctx, client)
}

func NonInteractiveAgreeToS(ctx context.Context, client client.Client) error {
	if DoDryRun {
		return ErrDryRun
	}

	if err := client.AgreeToS(ctx); err != nil {
		return err
	}
	term.Info("You have agreed to the Defang terms of service")
	return nil
}
