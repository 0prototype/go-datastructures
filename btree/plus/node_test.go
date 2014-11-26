package plus

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func init() {
	log.Print(`hate this`)
}

func constructMockPayloads(num uint64) keys {
	keys := make(keys, 0, num)
	for i := uint64(0); i < num; i++ {
		keys = append(keys, newPayload(newMockKey(i, i)))
	}

	return keys
}

func constructMockKeys(num uint64) keys {
	keys := make(keys, 0, num)

	for i := uint64(0); i < num; i++ {
		keys = append(keys, newMockKey(i, i))
	}

	return keys
}

func constructMockNodes(num uint64) nodes {
	nodes := make(nodes, 0, num)
	for i := uint64(0); i < num; i++ {
		keys := make(keys, 0, num)
		for j := uint64(0); j < num; j++ {
			keys = append(keys, newPayload(newMockKey(j*i+j, j*i+j)))
		}

		node := &lnode{
			keys: keys,
		}
		nodes = append(nodes, node)
		if i > 0 {
			nodes[i-1].(*lnode).pointer = node
		}
	}

	return nodes
}

func constructMockInternalNode(nodes nodes) *inode {
	if len(nodes) < 2 {
		return nil
	}

	keys := make(keys, 0, len(nodes)-1)
	for i := 1; i < len(nodes); i++ {
		keys = append(keys, nodes[i].(*lnode).keys[0].(*payload).key())
	}

	in := &inode{
		keys:  keys,
		nodes: nodes,
	}
	return in
}

func TestLeafNodeInsert(t *testing.T) {
	tree := newBTree(3)
	n := newLeafNode(3)
	key := newMockKey(3, 3)

	n.insert(tree, key)

	assert.Len(t, n.keys, 1)
	assert.Nil(t, n.pointer)
	assert.Equal(t, n.keys[0].(*payload).keys[0], key)
	assert.Equal(t, 0, n.keys[0].Compare(key))
}

func TestDuplicateLeafNodeInsert(t *testing.T) {
	tree := newBTree(3)
	n := newLeafNode(3)
	k1 := newMockKey(3, 3)
	k2 := newMockKey(3, 4)

	assert.True(t, n.insert(tree, k1))
	assert.True(t, n.insert(tree, k2))
	assert.False(t, n.insert(tree, k1))

	assert.Len(t, n.keys, 1)
	assert.Nil(t, n.pointer)
	if !assert.Len(t, n.keys[0].(*payload).keys, 2) {
		return
	}
	assert.Equal(t, n.keys[0].(*payload).keys[0], k1)
	assert.Equal(t, n.keys[0].(*payload).keys[1], k2)
	assert.Equal(t, 0, n.keys[0].Compare(k1))
	assert.Equal(t, 0, n.keys[0].Compare(k2))
}

func TestMultipleLeafNodeInsert(t *testing.T) {
	tree := newBTree(3)
	n := newLeafNode(3)

	k1 := newMockKey(3, 3)
	k2 := newMockKey(4, 4)

	assert.True(t, n.insert(tree, k1))
	n.insert(tree, k2)

	if !assert.Len(t, n.keys, 2) {
		return
	}
	assert.Nil(t, n.pointer)
	assert.Equal(t, k1, n.keys[0].(*payload).keys[0])
	assert.Equal(t, k2, n.keys[1].(*payload).keys[0])
}

func TestLeafNodeSplitEvenNumber(t *testing.T) {
	keys := constructMockPayloads(4)

	node := &lnode{
		keys: keys,
	}

	key, left, right := node.split()
	assert.Equal(t, keys[2].(*payload).key(), key)
	assert.Equal(t, left.(*lnode).keys, keys[:2])
	assert.Equal(t, right.(*lnode).keys, keys[2:])
	assert.Equal(t, left.(*lnode).pointer, right)
}

func TestLeafNodeSplitOddNumber(t *testing.T) {
	keys := constructMockPayloads(3)

	node := &lnode{
		keys: keys,
	}

	key, left, right := node.split()
	assert.Equal(t, keys[1].(*payload).key(), key)
	assert.Equal(t, left.(*lnode).keys, keys[:1])
	assert.Equal(t, right.(*lnode).keys, keys[1:])
	assert.Equal(t, left.(*lnode).pointer, right)
}

func TestTwoKeysLeafNodeSplit(t *testing.T) {
	keys := constructMockPayloads(2)

	node := &lnode{
		keys: keys,
	}

	key, left, right := node.split()
	assert.Equal(t, keys[1].(*payload).key(), key)
	assert.Equal(t, left.(*lnode).keys, keys[:1])
	assert.Equal(t, right.(*lnode).keys, keys[1:])
	assert.Equal(t, left.(*lnode).pointer, right)
}

func TestLessThanTwoKeysSplit(t *testing.T) {
	keys := constructMockPayloads(1)

	node := &lnode{
		keys: keys,
	}

	key, left, right := node.split()
	assert.Nil(t, key)
	assert.Nil(t, left)
	assert.Nil(t, right)
}

func TestInternalNodeSplit2_3_4(t *testing.T) {
	nodes := constructMockNodes(4)
	in := constructMockInternalNode(nodes)

	key, left, right := in.split()
	assert.Equal(t, nodes[3].(*lnode).keys[0].(*payload).key(), key)
	assert.Len(t, left.(*inode).keys, 1)
	assert.Len(t, right.(*inode).keys, 1)
	assert.Equal(t, nodes[:2], left.(*inode).nodes)
	assert.Equal(t, nodes[2:], right.(*inode).nodes)
}

func TestInternalNodeSplit3_4_5(t *testing.T) {
	nodes := constructMockNodes(5)
	in := constructMockInternalNode(nodes)

	key, left, right := in.split()
	assert.Equal(t, nodes[4].(*lnode).keys[0].(*payload).key(), key)
	assert.Len(t, left.(*inode).keys, 2)
	assert.Len(t, right.(*inode).keys, 1)
	assert.Equal(t, nodes[:3], left.(*inode).nodes)
	assert.Equal(t, nodes[3:], right.(*inode).nodes)
}

func TestInternalNodeLessThan3Keys(t *testing.T) {
	nodes := constructMockNodes(2)
	in := constructMockInternalNode(nodes)

	key, left, right := in.split()
	assert.Nil(t, key)
	assert.Nil(t, left)
	assert.Nil(t, right)
}
