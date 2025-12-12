package elicitations

import (
	"context"
	"fmt"
)

type Controller interface {
	RequestString(ctx context.Context, message, field string) (string, error)
	RequestStringWithDefault(ctx context.Context, message, field, defaultValue string) (string, error)
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
	Message string
	Schema  map[string]any
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

func (c *controller) RequestString(ctx context.Context, message, field string) (string, error) {
	return c.requestField(ctx, message, field, map[string]any{
		"type":        "string",
		"description": field,
	})
}

func (c *controller) RequestStringWithDefault(ctx context.Context, message, field, defaultValue string) (string, error) {
	return c.requestField(ctx, message, field, map[string]any{
		"type":        "string",
		"description": field,
		"default":     defaultValue,
	})
}

func (c *controller) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	return c.requestField(ctx, message, field, map[string]any{
		"type":        "string",
		"description": field,
		"enum":        options,
	})
}

func (c *controller) requestField(ctx context.Context, message, field string, schema map[string]any) (string, error) {
	response, err := c.client.Request(ctx, Request{
		Message: message,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				field: schema,
			},
			"required": []string{field},
		},
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
