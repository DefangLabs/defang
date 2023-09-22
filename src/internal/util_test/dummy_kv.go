package util_test

import (
	"time"

	"github.com/nats-io/nats.go"
)

type dummyKV map[string]*dummyKVE

func NewDummyKV() dummyKV {
	return dummyKV{}
}

func (d dummyKV) Get(key string) (nats.KeyValueEntry, error) {
	kve, ok := d[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return kve, nil
}

func (d dummyKV) Update(key string, value []byte, last uint64) (uint64, error) {
	kve, ok := d[key]
	if !ok {
		return 0, nats.ErrKeyNotFound
	} else if kve.Revision() != last {
		return 0, nats.ErrBadRequest
	}
	kve.revs = append(kve.revs, value)
	return kve.Revision(), nil
}

func (d dummyKV) Create(key string, value []byte) (revision uint64, err error) {
	if _, ok := d[key]; ok {
		return 0, nats.ErrKeyExists
	}
	d[key] = &dummyKVE{key: key, revs: [][]byte{value}}
	return 1, nil
}

func (d dummyKV) Put(key string, value []byte) (revision uint64, err error) {
	kve, ok := d[key]
	if !ok {
		return d.Create(key, value)
	}
	return d.Update(key, value, kve.Revision())
}

func (d dummyKV) GetRevision(key string, revision uint64) (nats.KeyValueEntry, error) {
	kve, ok := d[key]
	if !ok || revision > kve.Revision() {
		return nil, nats.ErrKeyNotFound
	}
	return &dummyKVE{key: key, revs: kve.revs[0:revision]}, nil
}

func (d dummyKV) PutString(key string, value string) (uint64, error) {
	return d.Put(key, []byte(value))
}

func (dummyKV) Delete(key string, opts ...nats.DeleteOpt) error                    { panic("no impl") }
func (dummyKV) Purge(key string, opts ...nats.DeleteOpt) error                     { panic("no impl") }
func (dummyKV) Watch(keys string, opts ...nats.WatchOpt) (nats.KeyWatcher, error)  { panic("no impl") }
func (dummyKV) WatchAll(opts ...nats.WatchOpt) (nats.KeyWatcher, error)            { panic("no impl") }
func (dummyKV) Keys(opts ...nats.WatchOpt) ([]string, error)                       { panic("no impl") }
func (dummyKV) History(k string, o ...nats.WatchOpt) ([]nats.KeyValueEntry, error) { panic("no impl") }
func (dummyKV) Bucket() string                                                     { panic("no impl") }
func (dummyKV) PurgeDeletes(opts ...nats.PurgeOpt) error                           { panic("no impl") }
func (dummyKV) Status() (nats.KeyValueStatus, error)                               { panic("no impl") }

type dummyKVE struct {
	key  string
	revs [][]byte
}

func (d dummyKVE) Key() string              { return d.key }
func (d dummyKVE) Value() []byte            { return d.revs[len(d.revs)-1] }
func (d dummyKVE) Revision() uint64         { return uint64(len(d.revs)) }
func (dummyKVE) Bucket() string             { panic("no impl") }
func (dummyKVE) Created() time.Time         { panic("no impl") }
func (dummyKVE) Delta() uint64              { panic("no impl") }
func (dummyKVE) Operation() nats.KeyValueOp { panic("no impl") }
