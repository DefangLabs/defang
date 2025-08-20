package login

import (
	"context"
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
)

var ErrTermsNotAgreed = errors.New("you must agree to the Defang terms of service to use this tool")

var P = track.P

func InteractiveAgreeToS(ctx context.Context, c client.FabricClient) error {
	if client.TermsAccepted() {
		// The user has already agreed to the terms of service recently
		if err := nonInteractiveAgreeToS(ctx, c); err != nil {
			term.Debug("unable to agree to terms:", err) // not fatal
		}
		return nil
	}

	term.Println("Our latest terms of service can be found at https://s.defang.io/tos")

	var agreeToS bool
	err := survey.AskOne(&survey.Confirm{
		Message: "Do you agree to the Defang terms of service?",
		Help:    "You must agree to the Defang terms of service to continue using this tool",
	}, &agreeToS, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return err
	}

	track.Evt("AgreeToS", P("agree", agreeToS))
	if !agreeToS {
		return ErrTermsNotAgreed
	}

	return NonInteractiveAgreeToS(ctx, c)
}

func NonInteractiveAgreeToS(ctx context.Context, c client.FabricClient) error {
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
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
