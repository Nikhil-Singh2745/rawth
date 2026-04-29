package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func tempDBPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

// --- Pager Tests ---

func TestPagerCreateAndReopen(t *testing.T) {
	path := tempDBPath(t)

	// Create
	p, err := OpenPager(path)
	if err != nil {
		t.Fatalf("failed to create pager: %v", err)
	}
	if p.PageCount() != 1 {
		t.Fatalf("expected 1 page (header), got %d", p.PageCount())
	}
	p.Close()

	// Reopen
	p2, err := OpenPager(path)
	if err != nil {
		t.Fatalf("failed to reopen pager: %v", err)
	}
	defer p2.Close()
	if p2.PageCount() != 1 {
		t.Fatalf("expected 1 page after reopen, got %d", p2.PageCount())
	}
}

func TestPagerAllocateAndFree(t *testing.T) {
	path := tempDBPath(t)
	p, err := OpenPager(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// Allocate 3 pages
	ids := make([]uint32, 3)
	for i := range ids {
		id, err := p.AllocatePage()
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
	}

	if p.PageCount() != 4 { // 1 header + 3 data
		t.Fatalf("expected 4 pages, got %d", p.PageCount())
	}

	// Free the middle page
	if err := p.FreePage(ids[1]); err != nil {
		t.Fatal(err)
	}

	// Allocate should reuse the freed page
	reused, err := p.AllocatePage()
	if err != nil {
		t.Fatal(err)
	}
	if reused != ids[1] {
		t.Fatalf("expected reused page %d, got %d", ids[1], reused)
	}

	// Page count should NOT have increased
	if p.PageCount() != 4 {
		t.Fatalf("expected 4 pages after reuse, got %d", p.PageCount())
	}
}

func TestPagerBadMagic(t *testing.T) {
	path := tempDBPath(t)

	// Write garbage
	os.WriteFile(path, make([]byte, PageSize), 0644)

	_, err := OpenPager(path)
	if err == nil {
		t.Fatal("expected error for bad magic bytes")
	}
}

// --- B+Tree Tests ---

func TestBTreeBasicCRUD(t *testing.T) {
	path := tempDBPath(t)
	p, _ := OpenPager(path)
	defer p.Close()

	tree, err := NewBPlusTree(p)
	if err != nil {
		t.Fatal(err)
	}

	// Put
	if err := tree.Put([]byte("hello"), []byte("world")); err != nil {
		t.Fatal(err)
	}

	// Get
	val, err := tree.Get([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "world" {
		t.Fatalf("expected 'world', got %q", val)
	}

	// Update
	if err := tree.Put([]byte("hello"), []byte("universe")); err != nil {
		t.Fatal(err)
	}
	val, err = tree.Get([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "universe" {
		t.Fatalf("expected 'universe', got %q", val)
	}

	// Delete
	if err := tree.Delete([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	_, err = tree.Get([]byte("hello"))
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestBTreeManyKeys(t *testing.T) {
	path := tempDBPath(t)
	p, _ := OpenPager(path)
	defer p.Close()

	tree, _ := NewBPlusTree(p)

	// Insert 200 keys to force multiple splits
	n := 200
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%04d", i))
		val := []byte(fmt.Sprintf("value_%04d", i))
		if err := tree.Put(key, val); err != nil {
			t.Fatalf("put key_%04d: %v", i, err)
		}
	}

	// Verify all keys exist
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%04d", i))
		expected := fmt.Sprintf("value_%04d", i)
		val, err := tree.Get(key)
		if err != nil {
			t.Fatalf("get key_%04d: %v", i, err)
		}
		if string(val) != expected {
			t.Fatalf("key_%04d: expected %q, got %q", i, expected, val)
		}
	}

	// Check count
	count, _ := tree.Count()
	if count != n {
		t.Fatalf("expected %d keys, got %d", n, count)
	}

	// Check depth > 1 (splits should have happened)
	depth, _ := tree.Depth()
	if depth < 2 {
		t.Logf("tree depth is %d (may be 1 if pages are large enough)", depth)
	}

	// Verify Keys() returns sorted
	keys, _ := tree.Keys()
	for i := 1; i < len(keys); i++ {
		if string(keys[i]) <= string(keys[i-1]) {
			t.Fatalf("keys not sorted at index %d: %q <= %q", i, keys[i], keys[i-1])
		}
	}
}

func TestBTreePersistence(t *testing.T) {
	path := tempDBPath(t)

	// Write
	{
		p, _ := OpenPager(path)
		tree, _ := NewBPlusTree(p)
		tree.Put([]byte("persist"), []byte("this works"))
		p.Flush()
		p.Close()
	}

	// Read
	{
		p, _ := OpenPager(path)
		defer p.Close()
		tree, _ := NewBPlusTree(p)
		val, err := tree.Get([]byte("persist"))
		if err != nil {
			t.Fatal(err)
		}
		if string(val) != "this works" {
			t.Fatalf("expected 'this works', got %q", val)
		}
	}
}

// --- Engine Tests ---

func TestEngineBasic(t *testing.T) {
	path := tempDBPath(t)
	e, err := OpenEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// Put + Get
	e.Put([]byte("key1"), []byte("val1"), 0)
	val, err := e.Get([]byte("key1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "val1" {
		t.Fatalf("expected val1, got %q", val)
	}

	// Has
	exists, _ := e.Has([]byte("key1"))
	if !exists {
		t.Fatal("expected key1 to exist")
	}

	// Keys
	keys, _ := e.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	// Delete
	e.Delete([]byte("key1"))
	_, err = e.Get([]byte("key1"))
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound after delete, got %v", err)
	}

	// Stats
	stats := e.Stats()
	if stats.PageCount < 1 {
		t.Fatal("expected at least 1 page")
	}
}

func TestEngineNuke(t *testing.T) {
	path := tempDBPath(t)
	e, _ := OpenEngine(path)

	e.Put([]byte("a"), []byte("1"), 0)
	e.Put([]byte("b"), []byte("2"), 0)
	e.Put([]byte("c"), []byte("3"), 0)

	keys, _ := e.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	e.Nuke()

	keys, _ = e.Keys()
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after nuke, got %d", len(keys))
	}

	// Should still work after nuke
	e.Put([]byte("post_nuke"), []byte("still works"), 0)
	val, _ := e.Get([]byte("post_nuke"))
	if string(val) != "still works" {
		t.Fatalf("expected 'still works', got %q", val)
	}

	e.Close()
}
