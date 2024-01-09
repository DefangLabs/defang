//go:build integration

package client

import (
	"context"
	"strings"
	"testing"

	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func TestDeploy(t *testing.T) {
	b := NewByocAWS("ten ant", "")

	t.Run("multiple ingress without domain", func(t *testing.T) {
		_, err := b.Deploy(context.TODO(), &v1.DeployRequest{
			Services: []*v1.Service{{
				Name:  "test",
				Image: "docker.io/library/nginx:latest",
				Ports: []*v1.Port{{
					Target: 80,
					Mode:   v1.Mode_INGRESS,
				}, {
					Target: 443,
					Mode:   v1.Mode_INGRESS,
				}},
			}},
		})
		if err == nil || !strings.Contains(err.Error(), "duplicate endpoint:") {
			t.Error("expected error")
		}
	})
}

func TestTail(t *testing.T) {
	b := NewByocAWS("tenant", "")

	ss, err := b.Tail(context.TODO(), &v1.TailRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()

	// First we expect "true" (the "start" event)
	if ss.Receive() != true {
		t.Error("expected Receive() to return true")
	}
	if len(ss.Msg().Entries) != 0 {
		t.Error("expected empty entries")
	}
	err = ss.Err()
	if err != nil {
		t.Error(err)
	}
}
