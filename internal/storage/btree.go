package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// nodes are either full of data (leaf) or full of directions (internal)
const (
	NodeTypeInternal uint8 = 1
	NodeTypeLeaf     uint8 = 2
)

// okay so this is how we pack things into 4096 bytes 
// we have a small header and then just a big blob of cells 
// internal cells are like: go to this page if the key is bigger than this
// leaf cells are the actual gold: key and the value
const (
	nodeHeaderSize = 7  // type(1) + key_count(2) + right_ptr(4)
	maxKeySize     = 255
	maxValueSize   = 3840

	// 4089 bytes for the actual data. if we go over this we have to split 
	// splitting is the worst part of my life right now
	cellAreaSize = PageSize - nodeHeaderSize 
)

// bplus tree... everything is on disk. no pointers. only page ids. 
// if you mess up one byte the whole tree becomes a forest fire
type BPlusTree struct {
	pager *Pager
}

// starts the tree. if page root is 0 it means we are brand new 
// so we make a root leaf and call it a day
func NewBPlusTree(pager *Pager) (*BPlusTree, error) {
	tree := &BPlusTree{pager: pager}

	if pager.RootPage() == 0 {
		rootID, err := pager.AllocatePage()
		if err != nil {
			return nil, fmt.Errorf("rawth: failed to create root node: %w", err)
		}

		root := makeLeafNode()
		if err := tree.writeNode(rootID, root); err != nil {
			return nil, err
		}

		if err := pager.SetRootPage(rootID); err != nil {
			return nil, err
		}
	}

	return tree, nil
}

// this is what a node looks like when we are actually touching it in memory 
// we read it from bytes, mess with it, and then flatten it back to bytes
type node struct {
	nodeType uint8
	keys     [][]byte
	values   [][]byte  // only if leaf
	children []uint32  // only if internal
	rightPtr uint32    // next leaf or rightmost child
}

func makeLeafNode() *node {
	return &node{
		nodeType: NodeTypeLeaf,
		keys:     make([][]byte, 0),
		values:   make([][]byte, 0),
		rightPtr: 0,
	}
}

func makeInternalNode() *node {
	return &node{
		nodeType: NodeTypeInternal,
		keys:     make([][]byte, 0),
		children: make([]uint32, 0),
		rightPtr: 0,
	}
}

// put something in. if it's already there we just swap the value. 
// if the root splits we have to make a new root which means the tree grows taller 
// i wish i grew taller as easily as this tree
func (t *BPlusTree) Put(key, value []byte) error {
	if len(key) == 0 {
		return errors.New("rawth: key cannot be empty")
	}
	if len(key) > maxKeySize {
		return fmt.Errorf("rawth: key too long (%d bytes, max %d)", len(key), maxKeySize)
	}
	if len(value) > maxValueSize {
		return fmt.Errorf("rawth: value too long (%d bytes, max %d)", len(value), maxValueSize)
	}

	rootID := t.pager.RootPage()

	newKey, newChildID, err := t.insert(rootID, key, value)
	if err != nil {
		return err
	}

	// root split! new level unlocked
	if newKey != nil {
		newRootID, err := t.pager.AllocatePage()
		if err != nil {
			return err
		}

		newRoot := makeInternalNode()
		newRoot.keys = append(newRoot.keys, newKey)
		newRoot.children = append(newRoot.children, rootID)
		newRoot.rightPtr = newChildID

		if err := t.writeNode(newRootID, newRoot); err != nil {
			return err
		}

		if err := t.pager.SetRootPage(newRootID); err != nil {
			return err
		}
	}

	return t.pager.Flush()
}

// recursion is my friend and my enemy. we go down until we hit a leaf.
func (t *BPlusTree) insert(pageID uint32, key, value []byte) ([]byte, uint32, error) {
	n, err := t.readNode(pageID)
	if err != nil {
		return nil, 0, err
	}

	if n.nodeType == NodeTypeLeaf {
		return t.insertLeaf(pageID, n, key, value)
	}
	return t.insertInternal(pageID, n, key, value)
}

func (t *BPlusTree) insertLeaf(pageID uint32, n *node, key, value []byte) ([]byte, uint32, error) {
	pos := 0
	for pos < len(n.keys) {
		cmp := bytes.Compare(key, n.keys[pos])
		if cmp == 0 {
			// already got it. just update and leave.
			n.values[pos] = value
			if err := t.writeNode(pageID, n); err != nil {
				return nil, 0, err
			}
			return nil, 0, nil
		}
		if cmp < 0 {
			break
		}
		pos++
	}

	n.keys = insertAt(n.keys, pos, copyBytes(key))
	n.values = insertAt(n.values, pos, copyBytes(value))

	// if the page is too heavy we split it. binary diet.
	if t.leafNodeSize(n) > cellAreaSize {
		return t.splitLeaf(pageID, n)
	}

	if err := t.writeNode(pageID, n); err != nil {
		return nil, 0, err
	}
	return nil, 0, nil
}

// finding the right child is just a loop. binary search would be faster 
// but my keys are small and i am lazy.
func (t *BPlusTree) insertInternal(pageID uint32, n *node, key, value []byte) ([]byte, uint32, error) {
	childIdx := 0
	for childIdx < len(n.keys) {
		if bytes.Compare(key, n.keys[childIdx]) < 0 {
			break
		}
		childIdx++
	}

	var childPageID uint32
	if childIdx < len(n.children) {
		childPageID = n.children[childIdx]
	} else {
		childPageID = n.rightPtr
	}

	splitKey, splitChildID, err := t.insert(childPageID, key, value)
	if err != nil {
		return nil, 0, err
	}

	if splitKey == nil {
		return nil, 0, nil
	}

	// child split so we get a new key to manage. ugh more work.
	n.keys = insertAt(n.keys, childIdx, splitKey)

	if childIdx < len(n.children) {
		n.children = insertAtUint32(n.children, childIdx+1, splitChildID)
	} else {
		oldRight := n.rightPtr
		n.children = append(n.children, oldRight)
		n.rightPtr = splitChildID
	}

	if t.internalNodeSize(n) > cellAreaSize {
		return t.splitInternal(pageID, n)
	}

	if err := t.writeNode(pageID, n); err != nil {
		return nil, 0, err
	}
	return nil, 0, nil
}

// cut the leaf in half. right half becomes a new page. 
// old page points to the new page. linked list vibes.
func (t *BPlusTree) splitLeaf(pageID uint32, n *node) ([]byte, uint32, error) {
	mid := len(n.keys) / 2

	newLeaf := makeLeafNode()
	newLeaf.keys = make([][]byte, len(n.keys[mid:]))
	copy(newLeaf.keys, n.keys[mid:])
	newLeaf.values = make([][]byte, len(n.values[mid:]))
	copy(newLeaf.values, n.values[mid:])
	newLeaf.rightPtr = n.rightPtr 

	newPageID, err := t.pager.AllocatePage()
	if err != nil {
		return nil, 0, err
	}

	n.keys = n.keys[:mid]
	n.values = n.values[:mid]
	n.rightPtr = newPageID 

	if err := t.writeNode(pageID, n); err != nil {
		return nil, 0, err
	}
	if err := t.writeNode(newPageID, newLeaf); err != nil {
		return nil, 0, err
	}

	splitKey := copyBytes(newLeaf.keys[0])
	return splitKey, newPageID, nil
}

// splitting internal nodes is slightly different because one key moves up 
// and doesn't stay in the children. b-trees are weird man.
func (t *BPlusTree) splitInternal(pageID uint32, n *node) ([]byte, uint32, error) {
	mid := len(n.keys) / 2
	promoteKey := copyBytes(n.keys[mid])

	newInternal := makeInternalNode()
	newInternal.keys = make([][]byte, len(n.keys[mid+1:]))
	copy(newInternal.keys, n.keys[mid+1:])

	if mid+1 < len(n.children) {
		newInternal.children = make([]uint32, len(n.children[mid+1:]))
		copy(newInternal.children, n.children[mid+1:])
	}
	newInternal.rightPtr = n.rightPtr

	newPageID, err := t.pager.AllocatePage()
	if err != nil {
		return nil, 0, err
	}

	n.keys = n.keys[:mid]
	n.children = n.children[:mid+1]
	if len(n.children) > 0 {
		n.rightPtr = n.children[len(n.children)-1]
		n.children = n.children[:len(n.children)-1]
	}

	if err := t.writeNode(pageID, n); err != nil {
		return nil, 0, err
	}
	if err := t.writeNode(newPageID, newInternal); err != nil {
		return nil, 0, err
	}

	return promoteKey, newPageID, nil
}

// get the value or die trying. usually just returns ErrKeyNotFound.
func (t *BPlusTree) Get(key []byte) ([]byte, error) {
	rootID := t.pager.RootPage()
	return t.search(rootID, key)
}

func (t *BPlusTree) search(pageID uint32, key []byte) ([]byte, error) {
	n, err := t.readNode(pageID)
	if err != nil {
		return nil, err
	}

	if n.nodeType == NodeTypeLeaf {
		for i, k := range n.keys {
			if bytes.Equal(k, key) {
				return copyBytes(n.values[i]), nil
			}
		}
		return nil, ErrKeyNotFound
	}

	childIdx := 0
	for childIdx < len(n.keys) {
		if bytes.Compare(key, n.keys[childIdx]) < 0 {
			break
		}
		childIdx++
	}

	var childPageID uint32
	if childIdx < len(n.children) {
		childPageID = n.children[childIdx]
	} else {
		childPageID = n.rightPtr
	}

	return t.search(childPageID, key)
}

// delete is simplified because merging nodes is a nightmare 
// and honestly life is too short for that. we just remove the key and move on. 
// if the node becomes empty? we dont care.
func (t *BPlusTree) Delete(key []byte) error {
	rootID := t.pager.RootPage()
	deleted, err := t.deleteFromNode(rootID, key)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrKeyNotFound
	}
	return t.pager.Flush()
}

func (t *BPlusTree) deleteFromNode(pageID uint32, key []byte) (bool, error) {
	n, err := t.readNode(pageID)
	if err != nil {
		return false, err
	}

	if n.nodeType == NodeTypeLeaf {
		for i, k := range n.keys {
			if bytes.Equal(k, key) {
				n.keys = removeAt(n.keys, i)
				n.values = removeAt(n.values, i)
				if err := t.writeNode(pageID, n); err != nil {
					return false, err
				}
				return true, nil
			}
		}
		return false, nil
	}

	childIdx := 0
	for childIdx < len(n.keys) {
		if bytes.Compare(key, n.keys[childIdx]) < 0 {
			break
		}
		childIdx++
	}

	var childPageID uint32
	if childIdx < len(n.children) {
		childPageID = n.children[childIdx]
	} else {
		childPageID = n.rightPtr
	}

	return t.deleteFromNode(childPageID, key)
}

func (t *BPlusTree) Has(key []byte) (bool, error) {
	_, err := t.Get(key)
	if err == ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// scan the leaves. linking leaves is the best thing about b+trees.
func (t *BPlusTree) Keys() ([][]byte, error) {
	rootID := t.pager.RootPage()

	leafID, err := t.findLeftmostLeaf(rootID)
	if err != nil {
		return nil, err
	}

	var allKeys [][]byte
	for leafID != 0 {
		n, err := t.readNode(leafID)
		if err != nil {
			return nil, err
		}
		for _, k := range n.keys {
			allKeys = append(allKeys, copyBytes(k))
		}
		leafID = n.rightPtr
	}

	return allKeys, nil
}

func (t *BPlusTree) ForEach(fn func(key, value []byte) error) error {
	rootID := t.pager.RootPage()
	leafID, err := t.findLeftmostLeaf(rootID)
	if err != nil {
		return err
	}

	for leafID != 0 {
		n, err := t.readNode(leafID)
		if err != nil {
			return err
		}
		for i, k := range n.keys {
			if err := fn(k, n.values[i]); err != nil {
				return err
			}
		}
		leafID = n.rightPtr
	}

	return nil
}

func (t *BPlusTree) Count() (int, error) {
	count := 0
	err := t.ForEach(func(key, value []byte) error {
		count++
		return nil
	})
	return count, err
}

func (t *BPlusTree) Depth() (int, error) {
	rootID := t.pager.RootPage()
	return t.depth(rootID)
}

func (t *BPlusTree) depth(pageID uint32) (int, error) {
	n, err := t.readNode(pageID)
	if err != nil {
		return 0, err
	}

	if n.nodeType == NodeTypeLeaf {
		return 1, nil
	}

	var childID uint32
	if len(n.children) > 0 {
		childID = n.children[0]
	} else {
		childID = n.rightPtr
	}

	d, err := t.depth(childID)
	if err != nil {
		return 0, err
	}
	return d + 1, nil
}

func (t *BPlusTree) findLeftmostLeaf(pageID uint32) (uint32, error) {
	n, err := t.readNode(pageID)
	if err != nil {
		return 0, err
	}

	if n.nodeType == NodeTypeLeaf {
		return pageID, nil
	}

	if len(n.children) > 0 {
		return t.findLeftmostLeaf(n.children[0])
	}
	return t.findLeftmostLeaf(n.rightPtr)
}

// --- Serializing things to disk. binary.LittleEndian is my god now. ---

func (t *BPlusTree) readNode(pageID uint32) (*node, error) {
	data, err := t.pager.ReadPage(pageID)
	if err != nil {
		return nil, err
	}
	return deserializeNode(data)
}

func (t *BPlusTree) writeNode(pageID uint32, n *node) error {
	data, err := serializeNode(n)
	if err != nil {
		return err
	}
	return t.pager.WritePage(pageID, data)
}

// this function is a giant mess of offsets. 
// if i forget to add 2 somewhere, the whole database is trash.
func serializeNode(n *node) ([]byte, error) {
	buf := make([]byte, PageSize)

	buf[0] = n.nodeType
	binary.LittleEndian.PutUint16(buf[1:3], uint16(len(n.keys)))
	binary.LittleEndian.PutUint32(buf[3:7], n.rightPtr)

	offset := nodeHeaderSize

	if n.nodeType == NodeTypeInternal {
		for i, key := range n.keys {
			if i < len(n.children) {
				if offset+4 > PageSize {
					return nil, errors.New("rawth: internal node overflow during serialization")
				}
				binary.LittleEndian.PutUint32(buf[offset:offset+4], n.children[i])
				offset += 4
			}

			if offset+2+len(key) > PageSize {
				return nil, errors.New("rawth: internal node overflow during serialization")
			}
			binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(len(key)))
			offset += 2
			copy(buf[offset:], key)
			offset += len(key)
		}
	} else {
		for i, key := range n.keys {
			val := n.values[i]

			needed := 2 + 2 + len(key) + len(val)
			if offset+needed > PageSize {
				return nil, errors.New("rawth: leaf node overflow during serialization")
			}

			binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(len(key)))
			offset += 2
			binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(len(val)))
			offset += 2
			copy(buf[offset:], key)
			offset += len(key)
			copy(buf[offset:], val)
			offset += len(val)
		}
	}

	return buf, nil
}

// taking raw bytes and turning them back into our node struct. 
// lots of bounds checking because i don't trust myself.
func deserializeNode(data []byte) (*node, error) {
	if len(data) < nodeHeaderSize {
		return nil, errors.New("rawth: page data too small for node header")
	}

	n := &node{
		nodeType: data[0],
		rightPtr: binary.LittleEndian.Uint32(data[3:7]),
	}

	keyCount := int(binary.LittleEndian.Uint16(data[1:3]))
	offset := nodeHeaderSize

	if n.nodeType == NodeTypeInternal {
		n.keys = make([][]byte, 0, keyCount)
		n.children = make([]uint32, 0, keyCount)

		for i := 0; i < keyCount; i++ {
			if offset+4 > len(data) {
				return nil, errors.New("rawth: truncated internal node (child pointer)")
			}
			childID := binary.LittleEndian.Uint32(data[offset : offset+4])
			n.children = append(n.children, childID)
			offset += 4

			if offset+2 > len(data) {
				return nil, errors.New("rawth: truncated internal node (key length)")
			}
			keyLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2

			if offset+keyLen > len(data) {
				return nil, errors.New("rawth: truncated internal node (key data)")
			}
			key := make([]byte, keyLen)
			copy(key, data[offset:offset+keyLen])
			n.keys = append(n.keys, key)
			offset += keyLen
		}
	} else if n.nodeType == NodeTypeLeaf {
		n.keys = make([][]byte, 0, keyCount)
		n.values = make([][]byte, 0, keyCount)

		for i := 0; i < keyCount; i++ {
			if offset+4 > len(data) {
				return nil, errors.New("rawth: truncated leaf node (key/val lengths)")
			}
			keyLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2
			valLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2

			if offset+keyLen+valLen > len(data) {
				return nil, errors.New("rawth: truncated leaf node (key/val data)")
			}

			key := make([]byte, keyLen)
			copy(key, data[offset:offset+keyLen])
			offset += keyLen

			val := make([]byte, valLen)
			copy(val, data[offset:offset+valLen])
			offset += valLen

			n.keys = append(n.keys, key)
			n.values = append(n.values, val)
		}
	} else {
		return nil, fmt.Errorf("rawth: unknown node type %d", n.nodeType)
	}

	return n, nil
}

// these just help us see if we are about to explode the page 
func (t *BPlusTree) leafNodeSize(n *node) int {
	size := 0
	for i, key := range n.keys {
		size += 2 + 2 + len(key) + len(n.values[i]) 
	}
	return size
}

func (t *BPlusTree) internalNodeSize(n *node) int {
	size := 0
	for i, key := range n.keys {
		size += 2 + len(key) 
		if i < len(n.children) {
			size += 4 
		}
	}
	return size
}

var ErrKeyNotFound = errors.New("rawth: key not found")

func copyBytes(b []byte) []byte {
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}

func insertAt[T any](slice []T, index int, value T) []T {
	slice = append(slice, value) 
	copy(slice[index+1:], slice[index:])
	slice[index] = value
	return slice
}

func insertAtUint32(slice []uint32, index int, value uint32) []uint32 {
	slice = append(slice, value)
	copy(slice[index+1:], slice[index:])
	slice[index] = value
	return slice
}

func removeAt[T any](slice []T, index int) []T {
	return append(slice[:index], slice[index+1:]...)
}
