package docker

import (
	"context"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	d := New()

	err := d.SetUp(context.TODO(), "alpine:latest", 6*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	defer d.TearDown(context.TODO())

	id, err := d.Run(context.TODO(), nil, "sh", "-c", "echo hello world")
	if err != nil {
		t.Fatal(err)
	}
	if id == nil || *id == "" {
		t.Fatal("id is empty")
	}

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second)
	defer cancel()
	err = d.Tail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
}
