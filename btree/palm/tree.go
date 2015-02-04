package palm

import (
	"sync"
	"sync/atomic"

	"github.com/Workiva/go-datastructures/queue"
)

type operation int

const (
	get operation = iota
	add
	remove
)

type recursiveBuild struct {
	keys   Keys
	nodes  nodes
	parent *node
}

type pending struct {
	bundles actions
	number  uint64
}

type bundleMap map[*node]actionBundles

type ptree struct {
	root                             *node
	ary, number, threads, bufferSize uint64
	pending                          *pending
	lock                             sync.RWMutex
	read, write                      sync.Mutex
}

func (ptree *ptree) runOperations() {
	ptree.lock.Lock()
	toPerform := ptree.pending
	ptree.pending = &pending{}
	ptree.lock.Unlock()

	q := queue.New(int64(ptree.number))
	var key Key
	var i uint64
	for _, ab := range toPerform.bundles {
		for {
			key, i = ab.getKey()
			if key == nil {
				break
			}

			q.Put(&actionBundle{key: key, index: i, action: ab})
		}
	}

	readOperations := make(bundleMap, q.Len())
	writeOperations := make(bundleMap, q.Len())

	queue.ExecuteInParallel(q, func(ifc interface{}) {
		ab := ifc.(*actionBundle)

		node := getParent(ptree.root, ab.key)
		ab.node = node
		switch ab.action.operation() {
		case get:
			ptree.read.Lock()
			readOperations[node] = append(readOperations[node], ab)
			ptree.read.Unlock()
		case add, remove:
			ptree.write.Lock()
			writeOperations[node] = append(writeOperations[node], ab)
			ptree.write.Unlock()
		}
	})
}

func (ptree *ptree) runReads(readOperations bundleMap) {
	q := queue.New(int64(len(readOperations)))

	for _, abs := range readOperations {
		for _, ab := range abs {
			q.Put(ab)
		}
	}

	queue.ExecuteInParallel(q, func(ifc interface{}) {
		ab := ifc.(*actionBundle)
		if ab.node == nil {
			ab.action.addResult(ab.index, nil)
			return
		}

		result := ab.node.search(ab.key)
		if result == len(ab.node.keys) {
			ab.action.addResult(ab.index, nil)
			return
		}

		if ab.node.keys[result].Compare(ab.key) == 0 {
			ab.action.addResult(ab.index, ab.node.keys[result])
			return
		}

		ab.action.addResult(ab.index, nil)
	})
}

func (ptree *ptree) recursiveSplit(n, parent *node, nodes *nodes, keys *Keys) {
	if !n.needsSplit(ptree.ary) {
		return
	}

	key, l, r := n.split()
	l.parent = parent
	r.parent = parent
	*keys = append(*keys, key)
	*nodes = append(*nodes, l, r)
	ptree.recursiveSplit(l, parent, nodes, keys)
	ptree.recursiveSplit(r, parent, nodes, keys)
}

func (ptree *ptree) recursiveAdd(layer map[*node][]*recursiveBuild, setRoot bool) {
	if len(layer) == 0 {
		if setRoot {
			panic(`ROOT HASN'T BEEN SET`)
		}
		return
	}

	if setRoot && len(layer) > 1 {
		panic(`SHOULD ONLY HAVE ONE ROOT`)
	}

	q := queue.New(int64(len(layer)))
	for _, rbs := range layer {
		q.Put(rbs)
	}

	layer = make(map[*node][]*recursiveBuild, len(layer))
	dummyRoot := &node{}
	queue.ExecuteInParallel(q, func(ifc interface{}) {
		rbs := ifc.([]*recursiveBuild)

		if len(rbs) == 0 {
			return
		}

		n := rbs[0].parent
		if setRoot {
			ptree.root = n
		}

		parent := n.parent
		if parent == nil {
			parent = dummyRoot
		}

		for _, rb := range rbs {
			for i, k := range rb.keys {
				if len(n.keys) == 0 {
					n.keys.insert(k)
					n.nodes.push(rb.nodes[i*2])
					n.nodes.push(rb.nodes[i*2+1])
					continue
				}

				index := n.search(k)
				n.keys.insertAt(k, i)
				n.nodes[index] = rb.nodes[i*2]
				n.nodes.insertAt(rb.nodes[i*2+1], index+1)
			}
		}

		if n.needsSplit(ptree.ary) {
			keys := make(Keys, 0, len(n.keys))
			nodes := make(nodes, 0, len(n.nodes))
			ptree.recursiveSplit(n, parent, &nodes, &keys)
			ptree.write.Lock()
			layer[parent] = append(
				layer[parent], &recursiveBuild{keys: keys, nodes: nodes, parent: parent},
			)
			ptree.write.Unlock()
		}
	})

	ptree.recursiveAdd(layer, setRoot)
}

func (ptree *ptree) runAdds(addOperations bundleMap) {
	q := queue.New(int64(len(addOperations)))

	for _, abs := range addOperations {
		q.Put(abs)
	}

	nextLayer := make(map[*node][]*recursiveBuild)
	dummyRoot := &node{} // constructed in case we need it
	var needRoot uint64
	queue.ExecuteInParallel(q, func(ifc interface{}) {
		abs := ifc.(actionBundles)

		if len(abs) == 0 {
			return
		}

		n := abs[0].node
		parent := n.parent
		if parent == nil {
			parent = dummyRoot
			atomic.AddUint64(&needRoot, 1)
		}

		for _, ab := range abs {
			oldKey := n.keys.insert(ab.key)
			ab.action.addResult(ab.index, oldKey)
		}

		if n.needsSplit(ptree.ary) {
			keys := make(Keys, 0, len(n.keys))
			nodes := make(nodes, 0, len(n.nodes))
			ptree.recursiveSplit(n, parent, &nodes, &keys)
			ptree.write.Lock()
			nextLayer[parent] = append(
				nextLayer[parent], &recursiveBuild{keys: keys, nodes: nodes, parent: parent},
			)
			ptree.write.Unlock()
		}
	})

	setRoot := needRoot > 0

	ptree.recursiveAdd(nextLayer, setRoot)
}
