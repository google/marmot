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

// Package storage provides Cogs with access to storage mechanisms that hold
// data that a Cog may need for operations.
package storage

// KeyValue allows a Cog to interact with Marmot's global key value store.
// This is for storing information needed by a Cog to operate, such as certs
// or passwords.  It should not store large data or data passed between Cogs
// in a Labor.
type Reader interface {
	// Read will allow retrieval of a value at key 'k' and store it in 'v'.
	// 'v' must be a pointer to a supportted type.
	// This can be an int, uint, float64, string or []byte.
	// A key cannot be larger than 1MiB.
	Read(cog, k string, v interface{}) error
}
