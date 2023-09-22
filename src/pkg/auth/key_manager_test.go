package auth

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/defang-io/defang/src/internal/util_test"
	natsServer "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
)

func TestKeyManager(t *testing.T) {
	kv := util_test.NewDummyKV()

	km := NewKeyManager(kv)
	key := km.getCurrentKey()
	if len(key.PrivateKey) == 0 {
		t.Error("expected private key")
	}
	if key.Kid == "" {
		t.Error("expected key ID")
	}
	if key.expiresAt.Before(time.Now()) {
		t.Error("expected expiry > now")
	}

	pub := km.GetPublicKey(key.Kid)
	if pub == nil {
		t.Error("expected key to be found")
	}

	if km.GetPublicKey("badkid") != nil {
		t.Error("expected key not to be found")
	}

	rotated := km.rotateKey(key)

	// race condition: another caller tries to rotate the same key
	rotate2 := km.rotateKey(key)
	if rotate2 != rotated {
		t.Error("expected key to be rotated only once")
	}

	key2 := km.getCurrentKey()
	if rotated != key2 {
		t.Fatal("expected key to be rotated")
	}
	if key2.Kid == key.Kid {
		t.Error("expected new key ID")
	}
	if key2.Equal(key) {
		t.Error("expected new key")
	}
	if key2.expiresAt.Before(key.expiresAt) {
		t.Error("expected new expiry > old")
	}

	if km.GetCurrentKey() != key2 {
		t.Error("expected key to be cached")
	}
	key2.expiresAt = time.Now().Add(-time.Second)
	if km.GetCurrentKey() == key2 {
		t.Error("expected key to be expired")
	}

	pub = km.GetPublicKey(key.Kid)
	if pub == nil {
		t.Fatal("expected key to be found")
	}
	if !key.Public().(ed25519.PublicKey).Equal(pub) {
		t.Fatal("expected keys to match")
	}

	pub = km.GetPublicKey("invalid")
	if pub != nil {
		t.Fatal("expected key to not be found")
	}
}

func TestWithRealNats(t *testing.T) {
	opts := natsServer.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natsServer.RunServer(&opts)
	defer s.Shutdown()

	nc, _ := nats.Connect(s.ClientURL())
	defer nc.Close()
	js, _ := nc.JetStream()
	kv, _ := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:  "keys",
		Storage: nats.MemoryStorage,
		History: nats.KeyValueMaxHistory,
	})

	km := NewKeyManager(kv)

	keys := make([]*Key, nats.KeyValueMaxHistory)
	for i := range keys {
		keys[i] = km.rotateKey(km.getCurrentKey())
	}

	for i := range keys {
		if km.GetPublicKey(keys[i].Kid) == nil {
			t.Errorf("expected key %d to be found", i)
		}
	}
}
