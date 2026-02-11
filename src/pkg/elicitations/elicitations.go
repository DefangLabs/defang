package elicitations

import (
	"context"
	"fmt"
)

type Validator func(any) error

type Options struct {
	DefaultValue string
	Validator    func(any) error
}

func WithValidator(validator func(any) error) func(*Options) {
	return func(opts *Options) {
		opts.Validator = validator
	}
}

func WithDefault(value string) func(*Options) {
	return func(opts *Options) {
		opts.DefaultValue = value
	}
}

type Controller interface {
	RequestStringWithOptions(ctx context.Context, message, field string, opts ...func(*Options)) (string, error)
	RequestEnum(ctx context.Context, message, field string, options []string) (string, error)
	SetSupported(supported bool)
	IsSupported() bool
}

type Client interface {
	Request(ctx context.Context, req Request) (Response, error)
}

type controller struct {
	client    Client
	supported bool
}

type Request struct {
	Message   string
	Schema    map[string]any
	Validator Validator
}

type Response struct {
	Action  string
	Content map[string]any
}

func NewController(client Client) Controller {
	return &controller{
		client:    client,
		supported: true,
	}
}

func (c *controller) RequestStringWithOptions(ctx context.Context, message, field string, opts ...func(*Options)) (string, error) {
	var options Options
	for _, opt := range opts {
		opt(&options)
	}
	schema := map[string]any{
		"type":        "string",
		"description": field,
	}
	if options.DefaultValue != "" {
		schema["default"] = options.DefaultValue
	}
	return c.requestField(ctx, message, field, schema, options.Validator)
}

func (c *controller) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	return c.requestField(ctx, message, field, map[string]any{
		"type":        "string",
		"description": field,
		"enum":        options,
	}, nil)
}

func (c *controller) requestField(ctx context.Context, message, field string, schema map[string]any, validator Validator) (string, error) {
	response, err := c.client.Request(ctx, Request{
		Message: message,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				field: schema,
			},
			"required": []string{field},
		},
		Validator: validator,
	})
	if err != nil {
		return "", fmt.Errorf("failed to elicit %s: %w", field, err)
	}
	value, ok := response.Content[field].(string)
	if !ok {
		return "", fmt.Errorf("invalid %s value", field)
	}

	return value, nil
}

func (c *controller) SetSupported(supported bool) {
	c.supported = supported
}

func (c *controller) IsSupported() bool {
	return c.supported
}
