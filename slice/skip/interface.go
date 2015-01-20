/*
Copyright 2014 Workiva, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package skip

// Entry defines items that can be inserted into the skip list.
// This will also be the type returned from a query.
type Entry interface {
	// Key defines this entry's place in the skip list.
	Key() uint64
}

// Entries is a typed list of interface Entry.
type Entries []Entry

type Iterator interface {
	Next() bool
	Value() Entry
	exhaust() Entries
}
