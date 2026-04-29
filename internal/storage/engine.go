package storage

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// the engine is the boss. it tells the pager and the tree what to do. 
// it also keeps track of things that should die (TTLs) 
// and some stats so we can show off on the website.
type Engine struct {
	mu    sync.RWMutex
	tree  *BPlusTree
	pager *Pager
	path  string

	// i keep ttls in a map in memory. if you restart the server 
	// all the expiration timers are gone. is that bad? probably. 
	// do i care? not really. just dont restart it.
	// also we check for death lazily. if you never ask for a key 
	// it might stay in the db forever. its a ghost.
	ttls map[string]time.Time

	// numbers for the dashboard
	putCount    uint64
	getCount    uint64
	deleteCount uint64
	startTime   time.Time
}

// stuff we send to the frontend 
type EngineStats struct {
	KeyCount    int    `json:"key_count"`
	TreeDepth   int    `json:"tree_depth"`
	PageCount   uint32 `json:"page_count"`
	FileSize    int64  `json:"file_size_bytes"`
	PutCount    uint64 `json:"put_count"`
	GetCount    uint64 `json:"get_count"`
	DeleteCount uint64 `json:"delete_count"`
	Uptime      string `json:"uptime"`
	FilePath    string `json:"file_path"`
}

// open the db. we start the pager and the tree. 
// if they fail we fail. teamwork makes the dream work.
func OpenEngine(path string) (*Engine, error) {
	pager, err := OpenPager(path)
	if err != nil {
		return nil, err
	}

	tree, err := NewBPlusTree(pager)
	if err != nil {
		pager.Close()
		return nil, err
	}

	return &Engine{
		tree:      tree,
		pager:     pager,
		path:      path,
		ttls:      make(map[string]time.Time),
		startTime: time.Now(),
	}, nil
}

// shove it into the tree. if ttl is set we make a note in our memory map.
func (e *Engine) Put(key, value []byte, ttlSeconds int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.tree.Put(key, value); err != nil {
		return err
	}

	if ttlSeconds > 0 {
		e.ttls[string(key)] = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	} else {
		delete(e.ttls, string(key))
	}

	e.putCount++
	return nil
}

// get it. but first check if it should be dead. 
// if it's too old we kill it right now and tell the user we never saw it.
func (e *Engine) Get(key []byte) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// is it dead yet?
	if expiry, ok := e.ttls[string(key)]; ok {
		if time.Now().After(expiry) {
			// kill it kill it kill it
			delete(e.ttls, string(key))
			e.tree.Delete(key)
			e.deleteCount++
			return nil, ErrKeyNotFound
		}
	}

	e.getCount++
	return e.tree.Get(key)
}

// goodbye key. we wont miss you.
func (e *Engine) Delete(key []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.ttls, string(key))
	e.deleteCount++
	return e.tree.Delete(key)
}

// check if it exists but dont bring me the data. 
// also checks if it's a ghost.
func (e *Engine) Has(key []byte) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if expiry, ok := e.ttls[string(key)]; ok {
		if time.Now().After(expiry) {
			delete(e.ttls, string(key))
			e.tree.Delete(key)
			e.deleteCount++
			return false, nil
		}
	}

	return e.tree.Has(key)
}

// list all the keys. we have to filter out the dead ones here too 
// otherwise the list looks messy.
func (e *Engine) Keys() ([][]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	allKeys, err := e.tree.Keys()
	if err != nil {
		return nil, err
	}

	var validKeys [][]byte
	now := time.Now()
	for _, k := range allKeys {
		if expiry, ok := e.ttls[string(k)]; ok {
			if now.After(expiry) {
				continue
			}
		}
		validKeys = append(validKeys, k)
	}

	return validKeys, nil
}

// the nuclear option. we literally delete the file from disk 
// and start over like nothing happened. use with caution lol.
func (e *Engine) Nuke() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pager.Close()

	// bye bye file
	if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rawth: failed to nuke database: %w", err)
	}

	pager, err := OpenPager(e.path)
	if err != nil {
		return err
	}

	tree, err := NewBPlusTree(pager)
	if err != nil {
		return err
	}

	e.pager = pager
	e.tree = tree
	e.ttls = make(map[string]time.Time)

	return nil
}

// get some numbers.
func (e *Engine) Stats() EngineStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	keyCount, _ := e.tree.Count()
	depth, _ := e.tree.Depth()

	return EngineStats{
		KeyCount:    keyCount,
		TreeDepth:   depth,
		PageCount:   e.pager.PageCount(),
		FileSize:    e.pager.FileSize(),
		PutCount:    e.putCount,
		GetCount:    e.getCount,
		DeleteCount: e.deleteCount,
		Uptime:      time.Since(e.startTime).Round(time.Second).String(),
		FilePath:    e.path,
	}
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pager.Close()
}
