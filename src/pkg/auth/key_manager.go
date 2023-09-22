package auth

import (
	"crypto"
	"crypto/ed25519"
	"log"
	"sync/atomic"
	"time"

	"github.com/defang-io/defang/src/pkg"
	"github.com/nats-io/nats.go"
)

type Key struct {
	ed25519.PrivateKey
	// *ecdsa.PrivateKey
	Kid       string
	expiresAt time.Time
}

const (
	kidLength = 36 // UUID v4
	validity  = 24 * time.Hour
)

type KeyManager struct {
	kv       nats.KeyValue
	_current atomic.Pointer[Key]
}

func NewKeyManager(kv nats.KeyValue) *KeyManager {
	km := &KeyManager{kv: kv}

	km.rotateKey(nil)
	return km
}

func (km *KeyManager) rotateKey(oldKey *Key) *Key {
	// Create an ephemeral keypair for this instance
	pub, pk, err := ed25519.GenerateKey(nil) //ecdsa.GenerateKey(secp256r1, rand.Reader)
	if err != nil {
		log.Panicln("failed to generate private key:", err)
	}

	// Store the public key in the NATS JetStream bucket (do this first, or clients with new JWTs will fail)
	// rev, err := km.kv.Put(natsKey, pub) //more efficient to use the revision as the KID
	kid := pkg.RandomID()
	_, err = km.kv.Create(kid, pub)
	if err != nil {
		log.Panicln("failed to store public key:", err)
	}

	// Set the current key (using memory fencing)
	newKey := &Key{PrivateKey: pk, Kid: kid, expiresAt: time.Now().Add(validity)}
	if km._current.CompareAndSwap(oldKey, newKey) {
		log.Println("new key", kid)
		return newKey // we won the race; return our new key
	} else {
		return km.getCurrentKey() // someone else won the race
	}
}

// GetPublicKey returns the public key with the given KID
func (km *KeyManager) GetPublicKey(kid string) crypto.PublicKey {
	// Avoid DoS attacks by checking the length of the KID
	if len(kid) > kidLength {
		return nil
	}
	// Hot path: check if the key is the current key
	current := km.getCurrentKey()
	if current.Kid == kid {
		return current.Public()
	}
	// Cold path: check if the key is in the NATS JetStream bucket
	// rev, _ := strconv.ParseUint(kid, 36, 64) //more efficient to use the revision as the KID
	// data, err := km.kv.GetRevision(natsKey, rev)
	data, err := km.kv.Get(kid)
	if err != nil {
		log.Printf("failed to get public key %s: %v\n", kid, err)
		return nil
	}
	return ed25519.PublicKey(data.Value())
}

func (km *KeyManager) getCurrentKey() *Key {
	return km._current.Load()
}

func (km *KeyManager) GetCurrentKey() *Key {
	currentKey := km.getCurrentKey()
	if time.Now().After(currentKey.expiresAt) {
		return km.rotateKey(currentKey)
	}
	return currentKey
}
