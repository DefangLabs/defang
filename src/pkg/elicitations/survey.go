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
	// for each property in the schema, create a survey question
	schemaPropMap, ok := req.Schema["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("invalid schema properties")
	}
	for key, prop := range schemaPropMap {
		propMap, ok := prop.(map[string]any)
		if !ok {
			return nil, errors.New("invalid property schema")
		}
		description, ok := propMap["description"].(string)
		if !ok {
			description = key
		}
		if propMap["enum"] != nil {
			enumValues, ok := propMap["enum"].([]string)
			if !ok {
				return nil, errors.New("invalid enum values")
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
	return questions, nil
}
