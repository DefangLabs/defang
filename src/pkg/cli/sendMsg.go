package cli

import (
	"context"
	"errors"

	"github.com/defang-io/defang/src/pkg/cli/client"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/google/uuid"
)

func SendMsg(ctx context.Context, client client.Client, subject, _type, id string, data []byte, contenttype string) error {
	if subject == "" {
		return errors.New("subject is required")
	}
	if _type == "" {
		return errors.New("type is required")
	}
	if id == "" {
		id = uuid.NewString()
	}

	Debug(" - Sending message to", subject, "with type", _type, "and id", id)

	if DoDryRun {
		return ErrDryRun
	}

	err := client.Publish(ctx, &defangv1.PublishRequest{Event: &defangv1.Event{
		Specversion:     "1.0",
		Type:            _type,
		Source:          "https://cli.defang.io",
		Subject:         subject,
		Id:              id,
		Datacontenttype: contenttype,
		Data:            data,
	}})
	return err
}
