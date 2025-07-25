package api

import (
	"reflect"
	"testing"
	"time"
)

func TestEmptyMemStore(t *testing.T) {
	var store = NewMemoryDataRequestStore()
	v, ok, err := store.Get(&StoreKey{Id: ""}, nil)
	if !reflect.DeepEqual(v, DataRequest{}) || ok || err != nil {
		t.Errorf("Get returned the wrong values for an empty store: %v, %v, %v instead of DataRequest{}, false, nil.", v, ok, err)
	}

	keys, err := store.List(&StoreKey{Id: "foo"}, nil)
	if len(keys) != 0 || err != nil {
		t.Errorf("List returned the wrong values for an empty store: %v, %v instead of empty slice, nil.", keys, err)
	}

	allKeys, err := store.ListAll(nil)
	if len(allKeys) != 0 || err != nil {
		t.Errorf("ListAll returned the wrong values for an empty store: %v, %v instead of empty slice, nil.", keys, err)
	}

	_, err = store.Put(&StoreKey{Id: ""}, DataRequest{}, nil)
	if err != nil {
		t.Errorf("Put errored when adding to an empty store: %v.", err)
	}
}

func TestOverrideMemStore(t *testing.T) {
	var store = NewMemoryDataRequestStore()

	store.Put(&StoreKey{Id: ""}, DataRequest{RegoQuery: "foo"}, nil)
	_, err := store.Put(&StoreKey{Id: ""}, DataRequest{RegoQuery: "bar"}, nil)
	if err != nil {
		t.Errorf("Put errored when overriding: %v.", err)
	}
	dr, _, _ := store.Get(&StoreKey{Id: ""}, nil)
	if dr.RegoQuery != "bar" {
		t.Errorf("Override unsuccessful: %v instead of bar.", dr.RegoQuery)
	}
}

func TestListMemStore(t *testing.T) {
	var store = NewMemoryDataRequestStore()
	store.Put(&StoreKey{Id: ""}, DataRequest{RegoQuery: ""}, nil)
	store.Put(&StoreKey{Id: "a"}, DataRequest{RegoQuery: "a"}, nil)
	store.Put(&StoreKey{Id: "ab"}, DataRequest{RegoQuery: "ab"}, nil)
	store.Put(&StoreKey{Id: "abc"}, DataRequest{RegoQuery: "abc"}, nil)
	store.Put(&StoreKey{Id: "abcd"}, DataRequest{RegoQuery: "abcd"}, nil)

	keys, err := store.List(&StoreKey{Id: "abc"}, nil)
	if err != nil || len(keys) != 2 {
		t.Errorf("Listing subset of store returned wrong values: %v, %v instead of slice containing abc and abcd, nil.", keys, err)
	}

	keys, err = store.List(&StoreKey{Id: "foobar"}, nil)
	if err != nil || len(keys) != 0 {
		t.Errorf("Listing subset of store returned wrong values: %v, %v instead of empty slice, nil.", keys, err)
	}

	keys, err = store.ListAll(nil)
	if err != nil || len(keys) != 5 {
		t.Errorf("Listing subset of store returned wrong values: %v, %v instead of slice containing \"\", a, ab, and abc, nil.", keys, err)
	}
}

func TestWatchMemStore(t *testing.T) {

	var store = NewMemoryDataRequestStore()

	ch := make(chan DataRequest)

	key := &StoreKey{Id: "foo"}
	etag := "some-etag"
	cb := func(dr DataRequest) {
		ch <- dr
	}

	found, err := store.Watch(key, "", 0, cb, nil)
	if err != nil {
		t.Fatal(err)
	}

	if found {
		t.Fatalf("unexpected key %v in store", key)
	}

	// insert key in store and add a watcher
	_, err = store.Put(key, DataRequest{RegoQuery: "foo", Etag: etag}, nil)
	if err != nil {
		t.Fatal(err)
	}

	found, err = store.Watch(key, "", 0, cb, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatalf("expected key %v in store", key)
	}

	msg := <-ch

	if msg.Etag != etag {
		t.Fatalf("expected etag for data request with key %v is %v but got %v", key, etag, msg.Etag)
	}

	// add a watcher for an existing key and etag
	found, err = store.Watch(key, etag, 1*time.Second, cb, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatalf("expected key %v in store", key)
	}

	msg = <-ch

	if msg.Etag != etag {
		t.Fatalf("expected etag for data request with key %v is %v but got %v", key, etag, msg.Etag)
	}

	// verify watcher is deleted
	if len(store.watchers) != 0 {
		t.Fatal("expected no registered watchers")
	}

	// add a watcher for an existing key and etag. Add a longer wait interval and send a new update before the wait
	// time expires
	found, err = store.Watch(key, etag, 5*time.Second, cb, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatalf("expected key %v in store", key)
	}

	done := make(chan struct{})
	go func() {
		select {
		case msg = <-ch:
			close(done)
		}
	}()

	etag2 := "some-etag2"
	_, err = store.Put(key, DataRequest{RegoQuery: "foo", Etag: etag2}, nil)
	if err != nil {
		t.Fatal(err)
	}

	<-done

	if msg.Etag != etag2 {
		t.Fatalf("expected etag for data request with key %v is %v but got %v", key, etag2, msg.Etag)
	}

	// verify watcher is deleted
	if len(store.watchers) != 0 {
		t.Fatal("expected no registered watchers")
	}
}

// NOTE: May want more extensive tests if this gets used more.
