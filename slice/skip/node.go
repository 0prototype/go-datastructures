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

import "log"

func init() {
	log.Printf(`I HATE THIS.`)
}

type nodes []*node

// reset will mark every index in this list of nodes as nil.
func (ns nodes) reset() nodes {
	for i := range ns {
		ns[i] = nil
	}

	return ns
}

func (ns nodes) search(key uint64, low, high int) int {
	var mid int
	for low < high {
		mid = (low + high) / 2
		switch node := ns[mid]; {
		case node == nil, node.key() > key:
			high = mid
		case node.key() <= key:
			low = mid + 1
		}
	}

	return mid
}

type node struct {
	// forward denotes the forward pointing pointers in this
	// node.
	forward nodes
	// entry is the associated value with this node.
	entry Entry
	keyu  uint64
}

// key is a helper method to return the key of the entry associated
// with this node.
func (n *node) key() uint64 {
	return n.keyu
}

// newNode will allocate and return a new node with the entry
// provided.  maxLevels will determine the length of the forward
// pointer list associated with this node.
func newNode(entry Entry, maxLevels uint8) *node {
	n := &node{
		entry:   entry,
		forward: make(nodes, maxLevels),
	}
	if entry != nil {
		n.keyu = entry.Key()
	}
	return n
}
