package ctrie

import (
	"bytes"
	"hash"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	w    = 5
	exp2 = 32
)

type Ctrie struct {
	root *iNode
	h    hash.Hash64
	hMu  sync.Mutex
}

type iNode struct {
	main *mainNode
}

// mainNode is either a cNode, tNode, or lNode.
type mainNode struct {
	cNode *cNode
	tNode *tNode
	lNode *lNode
}

type cNode struct {
	bmp   uint32
	array []branch
}

func newMainNode(x *sNode, xhc uint64, y *sNode, yhc uint64, lev uint) *mainNode {
	if lev < 35 {
		xidx := (xhc >> lev) & 0x1f
		yidx := (yhc >> lev) & 0x1f
		bmp := uint32((1 << xidx) | (1 << yidx))
		if xidx == yidx {
			main := newMainNode(x, xhc, y, yhc, lev+w)
			iNode := &iNode{main}
			return &mainNode{cNode: &cNode{bmp, []branch{iNode}}}
		}
		if xidx < yidx {
			return &mainNode{cNode: &cNode{bmp, []branch{x, y}}}
		}
		return &mainNode{cNode: &cNode{bmp, []branch{y, x}}}
	}
	return &mainNode{lNode: &lNode{sn: x, next: &lNode{sn: y}}}
}

// inserted returns a copy of this cNode with the new entry at the given
// position.
func (c *cNode) inserted(pos, flag uint32, br branch) *cNode {
	length := uint32(len(c.array))
	bmp := c.bmp
	array := make([]branch, length+1)
	for i := uint32(0); i < pos; i++ {
		array[i] = c.array[i]
	}
	array[pos] = br
	for i, x := pos, uint32(0); x < length-pos; i++ {
		array[i+1] = c.array[i]
		x++
	}
	ncn := &cNode{bmp: bmp | flag, array: array}
	return ncn
}

// updated returns a copy of this cNode with the entry at the given index
// updated.
func (c *cNode) updated(pos uint32, br branch) *cNode {
	array := make([]branch, len(c.array))
	for i, branch := range c.array {
		array[i] = branch
	}
	array[pos] = br
	ncn := &cNode{bmp: c.bmp, array: array}
	return ncn
}

// removed returns a copy of this cNode with the entry at the given index
// removed.
func (c *cNode) removed(pos, flag uint32) *cNode {
	length := uint32(len(c.array))
	bmp := c.bmp
	array := make([]branch, length-1)
	for i := uint32(0); i < pos; i++ {
		array[i] = c.array[i]
	}
	for i, x := pos, uint32(0); x < length-pos-1; i++ {
		array[i] = c.array[i+1]
		x++
	}
	ncn := &cNode{bmp: bmp ^ flag, array: array}
	return ncn
}

type tNode struct {
	*sNode
}

func (t *tNode) untombed() *sNode {
	return &sNode{&entry{key: t.key, hash: t.hash, value: t.value}}
}

type lNode struct {
	sn   *sNode
	next *lNode
}

// branch is either an iNode or sNode.
type branch interface{}

type entry struct {
	key   []byte
	hash  uint64
	value interface{}
}

type sNode struct {
	*entry
}

func New() *Ctrie {
	root := &iNode{main: &mainNode{cNode: &cNode{}}}
	return &Ctrie{root: root, h: fnv.New64a()}
}

func (c *Ctrie) SetHash(hash hash.Hash64) {
	c.hMu.Lock()
	c.h = hash
	c.hMu.Unlock()
}

func (c *Ctrie) Insert(key []byte, value interface{}) {
	c.insert(&entry{
		key:   key,
		hash:  c.hash(key),
		value: value,
	})
}

func (c *Ctrie) Lookup(key []byte) (interface{}, bool) {
	return c.lookup(&entry{key: key, hash: c.hash(key)})
}

func (c *Ctrie) Remove(key []byte) (interface{}, bool) {
	return c.remove(&entry{key: key, hash: c.hash(key)})
}

func (c *Ctrie) insert(entry *entry) {
	rootPtr := (*unsafe.Pointer)(unsafe.Pointer(&c.root))
	root := (*iNode)(atomic.LoadPointer(rootPtr))
	if !iinsert(root, entry, 0, nil) {
		c.insert(entry)
	}
}

func (c *Ctrie) lookup(entry *entry) (interface{}, bool) {
	rootPtr := (*unsafe.Pointer)(unsafe.Pointer(&c.root))
	root := (*iNode)(atomic.LoadPointer(rootPtr))
	result, exists, ok := ilookup(root, entry, 0, nil)
	for !ok {
		return c.lookup(entry)
	}
	return result, exists
}

func (c *Ctrie) remove(entry *entry) (interface{}, bool) {
	rootPtr := (*unsafe.Pointer)(unsafe.Pointer(&c.root))
	root := (*iNode)(atomic.LoadPointer(rootPtr))
	result, exists, ok := iremove(root, entry, 0, nil)
	for !ok {
		return c.remove(entry)
	}
	return result, exists
}

func (c *Ctrie) hash(k []byte) uint64 {
	c.hMu.Lock()
	c.h.Write(k)
	hash := c.h.Sum64()
	c.h.Reset()
	c.hMu.Unlock()
	return hash
}

func iinsert(i *iNode, entry *entry, lev uint, parent *iNode) bool {
	mainPtr := (*unsafe.Pointer)(unsafe.Pointer(&i.main))
	main := (*mainNode)(atomic.LoadPointer(mainPtr))
	switch {
	case main.cNode != nil:
		cn := main.cNode
		flag, pos := flagPos(entry.hash, lev, cn.bmp)
		if cn.bmp&flag == 0 {
			// If the relevant bit is not in the bitmap, then a copy of the
			// cNode with the new entry is created. The linearization point is
			// a successful CAS.
			ncn := &mainNode{cNode: cn.inserted(pos, flag, &sNode{entry})}
			return atomic.CompareAndSwapPointer(mainPtr, unsafe.Pointer(main), unsafe.Pointer(ncn))
		}
		// If the relevant bit is present in the bitmap, then its corresponding
		// branch is read from the array.
		branch := cn.array[pos]
		switch branch.(type) {
		case *iNode:
			// If the branch is an I-node, then iinsert is called recursively.
			return iinsert(branch.(*iNode), entry, lev+w, i)
		case *sNode:
			sn := branch.(*sNode)
			if !bytes.Equal(sn.key, entry.key) {
				// If the branch is an S-node and its key is not equal to the
				// key being inserted, then the Ctrie has to be extended with
				// an additional level. The C-node is replaced with its updated
				// version, created using the updated function that adds a new
				// I-node at the respective position. The new Inode has its
				// main node pointing to a C-node with both keys. The
				// linearization point is a successful CAS.
				nsn := &sNode{entry}
				nin := &iNode{newMainNode(sn, sn.hash, nsn, nsn.hash, lev+w)}
				ncn := &mainNode{cNode: cn.updated(pos, nin)}
				return atomic.CompareAndSwapPointer(mainPtr, unsafe.Pointer(main), unsafe.Pointer(ncn))
			}
			// If the key in the S-node is equal to the key being inserted,
			// then the C-node is replaced with its updated version with a new
			// S-node. The linearization point is a successful CAS.
			ncn := &mainNode{cNode: cn.updated(pos, &sNode{entry})}
			return atomic.CompareAndSwapPointer(mainPtr, unsafe.Pointer(main), unsafe.Pointer(ncn))
		default:
			panic("Ctrie is in an invalid state")
		}
	case main.tNode != nil:
		// TODO
		return true
	case main.lNode != nil:
		// TODO
		return true
	default:
		panic("Ctrie is in an invalid state")
	}
}

func ilookup(i *iNode, entry *entry, lev uint, parent *iNode) (interface{}, bool, bool) {
	mainPtr := (*unsafe.Pointer)(unsafe.Pointer(&i.main))
	// Linearization point.
	main := (*mainNode)(atomic.LoadPointer(mainPtr))
	switch {
	case main.cNode != nil:
		cn := main.cNode
		flag, pos := flagPos(entry.hash, lev, cn.bmp)
		if cn.bmp&flag == 0 {
			// If the bitmap does not contain the relevant bit, a key with the
			// required hashcode prefix is not present in the trie.
			return nil, false, true
		}
		// Otherwise, the relevant branch at index pos is read from the array.
		branch := cn.array[pos]
		switch branch.(type) {
		case *iNode:
			// If the branch is an I-node, the ilookup procedure is called
			// recursively at the next level.
			return ilookup(branch.(*iNode), entry, lev+w, i)
		case *sNode:
			// If the branch is an S-node, then the key within the S-node is
			// compared with the key being searched – these two keys have the
			// same hashcode prefixes, but they need not be equal. If they are
			// equal, the corresponding value from the S-node is
			// returned and a NOTFOUND value otherwise.
			sn := branch.(*sNode)
			if bytes.Equal(sn.key, entry.key) {
				return sn.value, true, true
			}
			return nil, false, true
		default:
			panic("Ctrie is in an invalid state")
		}
	case main.tNode != nil:
		// TODO
		return nil, false, true
	case main.lNode != nil:
		// TODO
		return nil, false, true
	default:
		panic("Ctrie is in an invalid state")
	}
}

func iremove(i *iNode, entry *entry, lev uint, parent *iNode) (interface{}, bool, bool) {
	mainPtr := (*unsafe.Pointer)(unsafe.Pointer(&i.main))
	// Linearization point.
	main := (*mainNode)(atomic.LoadPointer(mainPtr))
	switch {
	case main.cNode != nil:
		cn := main.cNode
		flag, pos := flagPos(entry.hash, lev, cn.bmp)
		if cn.bmp&flag == 0 {
			// If the bitmap does not contain the relevant bit, a key with the
			// required hashcode prefix is not present in the trie.
			return nil, false, true
		}
		// Otherwise, the relevant branch at index pos is read from the array.
		branch := cn.array[pos]
		switch branch.(type) {
		case *iNode:
			// If the branch is an I-node, the iremove procedure is called
			// recursively at the next level.
			return iremove(branch.(*iNode), entry, lev+w, i)
		case *sNode:
			// If the branch is an S-node, its key is compared against the key
			// being removed.
			sn := branch.(*sNode)
			if !bytes.Equal(sn.key, entry.key) {
				// If the keys are not equal, the NOTFOUND value is returned.
				return nil, false, true
			}
			//  If the keys are equal, a copy of the current node without the
			//  S-node is created. The contraction of the copy is then created
			//  using the toContracted procedure. A successful CAS will
			//  substitute the old C-node with the copied C-node, thus removing
			//  the S-node with the given key from the trie – this is the
			//  linearization point
			ncn := cn.removed(pos, flag)
			cntr := toContracted(ncn, lev)
			if atomic.CompareAndSwapPointer(mainPtr, unsafe.Pointer(main), unsafe.Pointer(cntr)) {
				if parent != nil {
					main = (*mainNode)(atomic.LoadPointer(mainPtr))
					if main.tNode != nil {
						cleanParent(parent, i, entry.hash, lev-w)
					}
				}
				return sn.value, true, true
			}
			return nil, false, false
		default:
			panic("Ctrie is in an invalid state")
		}
	case main.tNode != nil:
		// TODO
		return nil, false, true
	case main.lNode != nil:
		// TODO
		return nil, false, true
	default:
		panic("Ctrie is in an invalid state")
	}
}

func toContracted(cn *cNode, lev uint) *mainNode {
	if lev > 0 && len(cn.array) == 1 {
		branch := cn.array[0]
		switch branch.(type) {
		case *sNode:
			return &mainNode{tNode: &tNode{branch.(*sNode)}}
		default:
			return &mainNode{cNode: cn}
		}
	}
	return &mainNode{cNode: cn}
}

func toCompressed(cn *cNode, lev uint) *mainNode {
	bmp := cn.bmp
	i := 0
	arr := cn.array
	tmpArray := make([]branch, len(arr))
	for i < len(arr) {
		sub := arr[i]
		switch sub.(type) {
		case *iNode:
			inode := sub.(*iNode)
			mainPtr := (*unsafe.Pointer)(unsafe.Pointer(&inode.main))
			main := (*mainNode)(atomic.LoadPointer(mainPtr))
			tmpArray[i] = resurrect(inode, main)
		case *sNode:
			tmpArray[i] = sub
		default:
			panic("Ctrie is in an invalid state")
		}
		i++
	}

	return toContracted(&cNode{bmp: bmp, array: tmpArray}, lev)
}

func resurrect(iNode *iNode, main *mainNode) branch {
	if main.tNode != nil {
		return main.tNode.untombed()
	}
	return iNode
}

func clean(i *iNode, lev uint) bool {
	mainPtr := (*unsafe.Pointer)(unsafe.Pointer(&i.main))
	main := (*mainNode)(atomic.LoadPointer(mainPtr))
	if main.cNode != nil {
		return atomic.CompareAndSwapPointer(mainPtr, unsafe.Pointer(main),
			unsafe.Pointer(toCompressed(main.cNode, lev)))
	}
	return true
}

func cleanParent(p, i *iNode, hc uint64, lev uint) {
	var (
		mainPtr  = (*unsafe.Pointer)(unsafe.Pointer(&i.main))
		main     = (*mainNode)(atomic.LoadPointer(mainPtr))
		pMainPtr = (*unsafe.Pointer)(unsafe.Pointer(&p.main))
		pMain    = (*mainNode)(atomic.LoadPointer(pMainPtr))
	)
	if pMain.cNode != nil {
		flag, pos := flagPos(hc, lev, pMain.cNode.bmp)
		if pMain.cNode.bmp&flag != 0 {
			sub := pMain.cNode.array[pos]
			if sub == i && main.tNode != nil {
				ncn := pMain.cNode.updated(pos, resurrect(i, main))
				if !atomic.CompareAndSwapPointer(pMainPtr, unsafe.Pointer(pMain),
					unsafe.Pointer(toContracted(ncn, lev))) {
					cleanParent(p, i, hc, lev)
				}
			}
		}
	}
}

func flagPos(hashcode uint64, lev uint, bmp uint32) (uint32, uint32) {
	idx := (hashcode >> lev) & 0x1f
	flag := uint32(1) << uint32(idx)
	mask := uint32(flag - 1)
	pos := bitCount(bmp & mask)
	return flag, pos
}

func bitCount(x uint32) uint32 {
	x = ((x >> 1) & 0x55555555) + (x & 0x55555555)
	x = ((x >> 2) & 0x33333333) + (x & 0x33333333)
	x = ((x >> 4) & 0x0f0f0f0f) + (x & 0x0f0f0f0f)
	x = ((x >> 8) & 0x00ff00ff) + (x & 0x00ff00ff)
	return ((x >> 16) & 0x0000ffff) + (x & 0x0000ffff)
}
