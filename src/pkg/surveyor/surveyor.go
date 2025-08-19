package surveyor

import (
	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type Surveyor interface {
	AskOne(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error
}

type DefaultSurveyor struct {
	DefaultOpts []survey.AskOpt
}

func NewDefaultSurveyor() *DefaultSurveyor {
	return &DefaultSurveyor{
		DefaultOpts: []survey.AskOpt{survey.WithStdio(term.DefaultTerm.Stdio())},
	}
}

func (ds *DefaultSurveyor) AskOne(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	return survey.AskOne(prompt, response, append(ds.DefaultOpts, opts...)...)
}
