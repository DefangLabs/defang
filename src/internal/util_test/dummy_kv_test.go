package util_test

import (
	"testing"

	"github.com/nats-io/nats.go"
)

func TestDummyKV(t *testing.T) {
	kv := NewDummyKV()

	t.Run("Get non-existent", func(t *testing.T) {
		_, err := kv.Get("key")
		if err != nats.ErrKeyNotFound {
			t.Fatal("expected error ErrKeyNotFound, got:", err)
		}
	})

	t.Run("Update non-existent", func(t *testing.T) {
		_, err := kv.Update("key", []byte("value"), 0)
		if err != nats.ErrKeyNotFound {
			t.Fatal("expected error ErrKeyNotFound, got:", err)
		}
	})

	t.Run("Create new", func(t *testing.T) {
		rev, err := kv.Create("key", []byte("value"))
		if err != nil {
			t.Fatal(err)
		}
		if rev != 1 {
			t.Fatalf("expected revision 1, got %d", rev)
		}
	})

	t.Run("Update existing", func(t *testing.T) {
		rev, err := kv.Update("key", []byte("value"), 1)
		if err != nil {
			t.Fatal(err)
		}
		if rev != 2 {
			t.Fatalf("expected revision 1, got %d", rev)
		}
	})

	t.Run("Create existing", func(t *testing.T) {
		_, err := kv.Create("key", []byte("value"))
		if err != nats.ErrKeyExists {
			t.Fatal("expected error ErrKeyExists, got:", err)
		}
	})

	t.Run("Get existing", func(t *testing.T) {
		kve, err := kv.Get("key")
		if err != nil {
			t.Fatal(err)
		}
		if string(kve.Value()) != "value" {
			t.Fatalf("expected value, got %s", kve.Value())
		}
	})

	t.Run("Get another non-existent", func(t *testing.T) {
		_, err := kv.Get("another")
		if err != nats.ErrKeyNotFound {
			t.Fatal("expected error ErrKeyNotFound, got:", err)
		}
	})

	t.Run("Update wrong revision", func(t *testing.T) {
		_, err := kv.Update("key", []byte("value"), 0)
		if err != nats.ErrBadRequest {
			t.Fatal("expected error ErrBadRequest, got:", err)
		}
	})
}
