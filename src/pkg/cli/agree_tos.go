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

func InteractiveAgreeToS(ctx context.Context, c client.FabricClient) error {
	if client.TermsAccepted() {
		// The user has already agreed to the terms of service recently
		if err := nonInteractiveAgreeToS(ctx, c); err != nil {
			term.Debug("unable to agree to terms:", err) // not fatal
		}
		return nil
	}

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

	return NonInteractiveAgreeToS(ctx, c)
}

func NonInteractiveAgreeToS(ctx context.Context, c client.FabricClient) error {
	if DoDryRun {
		return ErrDryRun
	}

	// Persist the terms agreement in the state file so that we don't ask again
	if err := client.AcceptTerms(); err != nil {
		term.Debug("unable to persist terms agreement:", err) // not fatal
	}

	return nonInteractiveAgreeToS(ctx, c)
}

func nonInteractiveAgreeToS(ctx context.Context, c client.FabricClient) error {
	if err := c.AgreeToS(ctx); err != nil {
		return err
	}
	term.Info("You have agreed to the Defang terms of service")
	return nil
}
