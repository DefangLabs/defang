package tools

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type ElicitationsController interface {
	Request(context.Context, ElicitationRequest) (ElicitationResponse, error)
}

type ElicitationRequest struct {
	Message string
	Schema  map[string]any
}

type ElicitationResponse struct {
	Action  string
	Content map[string]string
}

type cliAgentElicitationsController struct {
	stdin  term.FileReader
	stdout term.FileWriter
	stderr io.Writer
}

func NewCLIAgentElicitationsController(stdin term.FileReader, stdout term.FileWriter, stderr io.Writer) *cliAgentElicitationsController {
	return &cliAgentElicitationsController{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
}

func (c *cliAgentElicitationsController) Request(_ context.Context, req ElicitationRequest) (ElicitationResponse, error) {
	var response = make(map[string]any, 0)
	questions := []*survey.Question{}
	// for each property in the schema, create a survey question
	schemaPropMap, ok := req.Schema["properties"].(map[string]any)
	if !ok {
		return ElicitationResponse{
			Action: "cancel",
		}, errors.New("invalid schema properties")
	}
	for key, prop := range schemaPropMap {
		propMap, ok := prop.(map[string]any)
		if !ok {
			return ElicitationResponse{
				Action: "cancel",
			}, errors.New("invalid property schema")
		}
		description, ok := propMap["description"].(string)
		if !ok {
			description = key
		}
		if propMap["enum"] != nil {
			enumValues, ok := propMap["enum"].([]string)
			if !ok {
				return ElicitationResponse{
					Action: "cancel",
				}, errors.New("invalid enum values")
			}
			options := []string{}
			for _, v := range enumValues {
				options = append(options, v)
			}
			prompt := &survey.Select{
				Message: description,
				Options: options,
			}
			question := &survey.Question{
				Name:   key,
				Prompt: prompt,
			}
			questions = append(questions, question)
		} else {
			inputPrompt := &survey.Input{
				Message: description,
			}
			if defaultValue, ok := propMap["default"].(string); ok {
				inputPrompt.Default = defaultValue
			}
			questions = append(questions, &survey.Question{
				Name:   key,
				Prompt: inputPrompt,
			})
		}
	}

	fmt.Fprintln(c.stdout, req.Message)
	err := survey.Ask(
		questions,
		&response,
		survey.WithStdio(c.stdin, c.stdout, c.stderr),
	)
	if err != nil {
		return ElicitationResponse{
			Action: "cancel",
		}, err
	}

	content := make(map[string]string, 0)
	for k, v := range response {
		answer, ok := v.(survey.OptionAnswer)
		if ok {
			content[k] = answer.Value
			continue
		}

		answerStr, ok := v.(string)
		if ok {
			content[k] = answerStr
			continue
		}

		return ElicitationResponse{
			Action: "cancel",
		}, errors.New("invalid answer type")
	}

	return ElicitationResponse{
		Action:  "accept",
		Content: content,
	}, nil
}
