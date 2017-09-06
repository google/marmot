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

package toml

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/golang/glog"
	"github.com/kylelemons/godebug/pretty"
)

func TestRead(t *testing.T) {
	myCog := "cog1"

	s := Store{
		myCog: Cog{
			"string": Value{
				Type:   String,
				String: "string",
			},
			"bytes": Value{
				Type:  Bytes,
				Bytes: []byte("hello"),
			},
			"int": Value{
				Type: Int,
				Int:  -2,
			},
			"uint": Value{
				Type: Uint,
				Uint: 2,
			},
			"float64": Value{
				Type:    Float64,
				Float64: 3.2,
			},
			"unknown": Value{
				Int: 2, // Notice that there is no Type declaration.
			},
		},
	}

	f, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("TestRead: %s", err)
	}
	defer os.Remove(f.Name())

	stat, err := f.Stat()
	if err != nil {
		t.Fatal(stat)
	}

	enc := toml.NewEncoder(f)
	if err = enc.Encode(s); err != nil {
		t.Fatalf("TestRead: %s", err)
	}
	f.Close()

	tests := []struct {
		desc string
		cog  string
		key  string
		put  interface{}
		want interface{}
		err  bool
	}{
		{
			desc: "Read string",
			cog:  myCog,
			key:  "string",
			put:  new(string),
			want: "string",
		},
		{
			desc: "Read bytes",
			cog:  myCog,
			key:  "bytes",
			put:  new([]byte),
			want: []byte("hello"),
		},
		{
			desc: "Read int",
			cog:  myCog,
			key:  "int",
			put:  new(int),
			want: -2,
		},
		{
			desc: "Read uint",
			cog:  myCog,
			key:  "uint",
			put:  new(uint),
			want: uint(2),
		},
		{
			desc: "Read float64",
			cog:  myCog,
			key:  "float64",
			put:  new(float64),
			want: 3.2,
		},
		{
			desc: "Error: Value is not the same type",
			cog:  myCog,
			key:  "int",
			put:  new(float64),
			err:  true,
		},
		{
			desc: "Error: v is not the same type",
			cog:  myCog,
			key:  "float64",
			put:  new(int),
			err:  true,
		},
		{
			desc: "Error: v is not a pointer type",
			cog:  myCog,
			key:  "int",
			put:  0,
			err:  true,
		},
		{
			desc: "Error: stored value has unknown type",
			cog:  myCog,
			key:  "unknown",
			put:  new(int),
			err:  true,
		},
	}

	kv, err := New(f.Name())
	if err != nil {
		t.Fatalf("TestRead: %s", err)
	}

	for _, test := range tests {
		err := kv.Read(test.cog, test.key, test.put)
		switch {
		case err == nil && test.err:
			t.Errorf("TestRead(%s): got err == nil, want err != nil", test.desc)
			continue
		case err != nil && !test.err:
			t.Errorf("TestRead(%s): got err == %s, want err == nil", test.desc, err)
			continue
		case err != nil:
			continue
		}

		if diff := pretty.Compare(test.want, test.put); diff != "" {
			t.Errorf("TestRead(%s): -want/+got:\n%s", test.desc, diff)
		}
	}

	// Make sure our updates can be seen.
	s["cog2"] = Cog{"hello": Value{Type: String, String: "world"}}

	f, err = os.OpenFile(f.Name(), os.O_TRUNC+os.O_WRONLY, 0600)
	if err != nil {
		t.Errorf("TestRead: could overwrite our test file: %s", err)
	}
	enc = toml.NewEncoder(f)
	if err = enc.Encode(s); err != nil {
		t.Fatalf("TestRead: %s", err)
	}
	f.Close()

	tooLong := time.Now().Add(20 * time.Second)
	myString := new(string)
	for {
		if time.Now().After(tooLong) {
			t.Fatalf("TestRead: never see the file change")
		}
		err := kv.Read("cog2", "hello", myString)
		if err != nil {
			glog.Infof("error when getting cog2 key 'hello': %s", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if *myString != "world" {
			t.Fatalf("TestRead: saw file change, but had wrong value: got %s, want %s", *myString, "world")
		}
		break
	}
}
