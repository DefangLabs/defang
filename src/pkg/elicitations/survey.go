package elicitations

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type surveyClient struct {
	stdin  term.FileReader
	stdout term.FileWriter
	stderr io.Writer
}

func NewSurveyClient(stdin term.FileReader, stdout term.FileWriter, stderr io.Writer) *surveyClient {
	return &surveyClient{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
}

func (c *surveyClient) Request(_ context.Context, req Request) (Response, error) {
	var response = make(map[string]any, 0)
	questions, err := prepareQuestions(req)
	if err != nil {
		return Response{
			Action: "cancel",
		}, err
	}

	fmt.Fprintln(c.stdout, req.Message)
	err = survey.Ask(
		questions,
		&response,
		survey.WithStdio(c.stdin, c.stdout, c.stderr),
	)
	if err != nil {
		return Response{
			Action: "cancel",
		}, err
	}

	content := make(map[string]any, 0)
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

		return Response{
			Action: "cancel",
		}, errors.New("invalid answer type")
	}

	return Response{
		Action:  "accept",
		Content: content,
	}, nil
}

func prepareQuestions(req Request) ([]*survey.Question, error) {
	questions := []*survey.Question{}
	schemaPropMap, ok := req.Schema["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("invalid schema properties")
	}
	for key, prop := range schemaPropMap {
		question, err := questionFromSchemaProp(key, prop)
		if err != nil {
			return nil, err
		}
		if req.Validator != nil {
			question.Validate = func(ans interface{}) error {
				return req.Validator(ans)
			}
		}
		questions = append(questions, question)
	}
	return questions, nil
}

func questionFromSchemaProp(key string, prop any) (*survey.Question, error) {
	propMap, ok := prop.(map[string]any)
	if !ok {
		return nil, errors.New("invalid property schema")
	}
	description, ok := propMap["description"].(string)
	if !ok {
		description = key
	}
	if propMap["enum"] != nil {
		var options []string
		switch enumValues := propMap["enum"].(type) {
		case []string:
			options = enumValues
		case []any:
			for _, v := range enumValues {
				s, ok := v.(string)
				if !ok {
					return nil, errors.New("invalid enum value type")
				}
				options = append(options, s)
			}
		default:
			return nil, errors.New("invalid enum values")
		}
		return &survey.Question{
			Name: key,
			Prompt: &survey.Select{
				Message: description,
				Options: options,
			},
		}, nil
	} else {
		inputPrompt := &survey.Input{
			Message: description,
		}
		if defaultValue, ok := propMap["default"].(string); ok {
			inputPrompt.Default = defaultValue
		}
		return &survey.Question{
			Name:   key,
			Prompt: inputPrompt,
		}, nil
	}
}
