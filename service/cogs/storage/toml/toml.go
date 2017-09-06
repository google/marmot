// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package toml implements a key/value store using TOML for Cogs to query.
package toml

import (
	"fmt"
	"os"
	"reflect"
	"sync/atomic"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	"github.com/google/marmot/service/cogs/storage"
)

// Store represents the TOML storage.
type Store map[string]Cog

// Cog represents Key/Values for Cogs.
type Cog map[string]Value

const (
	// Unknown indicates the Value Type is not known.
	Unknown = ""
	// String indicates the Value Type is a string.
	String = "string"
	// Int indicates the Value Type is an int.
	Int = "int"
	// Uint indicates the Value Type is an uint.
	Uint = "uint"
	// Float64 indicates the Value Type is a float64.
	Float64 = "float64"
	// Bytes indictes the Value Type is a []byte.
	Bytes = "bytes"
)

// Value represents a Value in a Cog map.
type Value struct {
	// Type is the type stored.
	Type string
	// Int stores an int.
	Int int
	// Uint stores an uint.
	Uint uint
	// Float64 stores a float64.
	Float64 float64
	// String stores a string.
	String string
	// Bytes stores a []byte.
	Bytes []byte
}

func (v Value) value() interface{} {
	switch v.Type {
	case Unknown:
		return nil
	case String:
		return v.String
	case Int:
		return v.Int
	case Uint:
		return v.Uint
	case Float64:
		return v.Float64
	case Bytes:
		return v.Bytes
	}
	return nil
}

// KeyValue implements a storage.Reader.
type KeyValue struct {
	store atomic.Value // *Store
}

// Read implements storage.Reader.Read().
func (kv *KeyValue) Read(cog, k string, v interface{}) error {
	s := kv.store.Load().(*Store)
	val := (*s)[cog][k]
	if val.Type == Unknown {
		return fmt.Errorf("key %q not found for cog %q", k, cog)
	}

	r := reflect.ValueOf(v)
	if r.Kind() != reflect.Ptr {
		return fmt.Errorf("v must be a pointer type, was %s", r.Kind())
	}

	if !r.Elem().CanSet() {
		return fmt.Errorf("v is a value that cannot be set, it is of type %T", v)
	}

	real := val.value()
	switch v.(type) {
	case *int:
		if _, ok := real.(int); !ok {
			return fmt.Errorf("cog(%s): could not put value at key %s into a %T, was of type %T", cog, k, v, real)
		}
		r.Elem().SetInt(int64(real.(int)))
	case *uint:
		if _, ok := real.(uint); !ok {
			return fmt.Errorf("cog(%s): could not put value at key %s into a %T, was of type %T", cog, k, v, real)
		}
		r.Elem().SetUint(uint64(real.(uint)))
	case *float64:
		if _, ok := real.(float64); !ok {
			return fmt.Errorf("cog(%s): could not put value at key %s into a %T, was of type %T", cog, k, v, real)
		}
		r.Elem().SetFloat(real.(float64))
	case *string:
		if _, ok := real.(string); !ok {
			return fmt.Errorf("cog(%s): could not put value at key %s into a %T, was of type %T", cog, k, v, real)
		}
		r.Elem().SetString(real.(string))
	case *[]byte:
		if _, ok := real.([]byte); !ok {
			return fmt.Errorf("cog(%s): could not put value at key %s into a %T, was of type %T", cog, k, v, real)
		}
		r.Elem().SetBytes(real.([]byte))
	default:
		return fmt.Errorf("the type passed %T is not a supported type", v)
	}
	return nil
}

func (kv *KeyValue) update(p string) error {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("can not create a filesystem watcher: %s", err)
	}

	if err := fs.Add(p); err != nil {
		return fmt.Errorf("cannot add path %s to filesystem watcher: %s", p, err)
	}

	go func() {
		for {
			select {
			case e := <-fs.Events:
				switch e.Op {
				case fsnotify.Rename, fsnotify.Write, fsnotify.Chmod, fsnotify.Create:
					s, err := readFile(p)
					if err != nil {
						glog.Errorf("the TOML key/value file changed to a file that has an error: %s", err)
						continue
					}
					kv.store.Store(s)
					glog.Infof("the TOML key/value file has been updated")
				case fsnotify.Remove:
					glog.Errorf("the TOML key/value file has been deleted!")
				default:
					glog.Infof("the TOML key/value store had a non-change filesystem event: %s", e.Op)
				}
			case e := <-fs.Errors:
				glog.Errorf("the TOML key/value store filewatcher had an error: %s", e)
			}
		}
	}()
	return nil
}

// New returns a new KeyValue using TOML as storage.
func New(p string) (storage.Reader, error) {
	s, err := readFile(p)
	if err != nil {
		return nil, err
	}

	kv := &KeyValue{}
	kv.store.Store(s)

	if err := kv.update(p); err != nil {
		return nil, err
	}

	return kv, nil
}

func readFile(p string) (*Store, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("could not open TOML KeyValue storage file at %s: %s", p, err)
	}

	if fi.Mode() != 0600 {
		return nil, fmt.Errorf("cannot use TOML KeyValue file at %s: file mode got %v, want 0600", p, fi.Mode())
	}

	s := &Store{}
	_, err = toml.DecodeFile(p, s)
	if err != nil {
		return nil, fmt.Errorf("problem decoding TOML KeyValue file at %s: %s", p, err)
	}
	return s, nil
}
