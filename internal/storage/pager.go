package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
)

// look 4kb is what linux likes and i like linux so it stays. dont ask to change it
const PageSize = 4096

// i named it RAWT because it sounds cool. if you see this in hex you know its my db
var MagicBytes = [4]byte{'R', 'A', 'W', 'T'}

const (
	FileVersion    = 1
	HeaderSize     = 32 // bytes used in the file header
	InvalidPageID  = 0  // page 0 is the boss page
	MaxBufferPages = 256
)

// this is the stuff at the start of the file. if this breaks we are doomed
type FileHeader struct {
	Magic        [4]byte // RAWT RAWT RAWT
	Version      uint32  
	PageSize     uint32  // 4096 or bust
	PageCount    uint32  // how many chunks we got
	RootPageID   uint32  // where the tree starts
	FreeListHead uint32  // pages we can recycle
}

// the pager is basically the middleman. it talks to the disk so the tree doesnt have to
// it also has a buffer map because reading from disk is slow and my laptop fan is already loud enough
type Pager struct {
	mu       sync.RWMutex
	file     *os.File
	header   FileHeader
	buffer   map[uint32][]byte // cache things here so we dont die of old age waiting for the hdd
	dirty    map[uint32]bool   // stuff that needs saving
	filePath string
}

// starts the whole thing. if the file is empty we make a new one. 
// if not we check for the RAWT bytes to make sure nobody gave us a jpeg or something
func OpenPager(path string) (*Pager, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("rawth: failed to open database file: %w", err)
	}

	p := &Pager{
		file:     file,
		buffer:   make(map[uint32][]byte),
		dirty:    make(map[uint32]bool),
		filePath: path,
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("rawth: failed to stat database file: %w", err)
	}

	if info.Size() == 0 {
		// fresh start
		if err := p.initNewFile(); err != nil {
			file.Close()
			return nil, err
		}
	} else {
		// hope the header isnt corrupted lol
		if err := p.readHeader(); err != nil {
			file.Close()
			return nil, err
		}
	}

	return p, nil
}

// make the first page. its just the header for now
func (p *Pager) initNewFile() error {
	p.header = FileHeader{
		Magic:        MagicBytes,
		Version:      FileVersion,
		PageSize:     PageSize,
		PageCount:    1, 
		RootPageID:   0, 
		FreeListHead: 0, 
	}
	return p.writeHeader()
}

// read the first 4096 bytes and pray
func (p *Pager) readHeader() error {
	buf := make([]byte, PageSize)
	_, err := p.file.ReadAt(buf, 0)
	if err != nil {
		return fmt.Errorf("rawth: failed to read file header: %w", err)
	}

	copy(p.header.Magic[:], buf[0:4])
	if p.header.Magic != MagicBytes {
		return errors.New("rawth: not a rawth database file (bad magic bytes)")
	}

	p.header.Version = binary.LittleEndian.Uint32(buf[4:8])
	p.header.PageSize = binary.LittleEndian.Uint32(buf[8:12])
	p.header.PageCount = binary.LittleEndian.Uint32(buf[12:16])
	p.header.RootPageID = binary.LittleEndian.Uint32(buf[16:20])
	p.header.FreeListHead = binary.LittleEndian.Uint32(buf[20:24])

	if p.header.Version != FileVersion {
		return fmt.Errorf("rawth: unsupported file version %d (expected %d)", p.header.Version, FileVersion)
	}

	if p.header.PageSize != PageSize {
		return fmt.Errorf("rawth: unexpected page size %d (expected %d)", p.header.PageSize, PageSize)
	}

	return nil
}

// save the header back. we do this a lot when we grow the file
func (p *Pager) writeHeader() error {
	buf := make([]byte, PageSize)

	copy(buf[0:4], p.header.Magic[:])
	binary.LittleEndian.PutUint32(buf[4:8], p.header.Version)
	binary.LittleEndian.PutUint32(buf[8:12], p.header.PageSize)
	binary.LittleEndian.PutUint32(buf[12:16], p.header.PageCount)
	binary.LittleEndian.PutUint32(buf[16:20], p.header.RootPageID)
	binary.LittleEndian.PutUint32(buf[20:24], p.header.FreeListHead)

	_, err := p.file.WriteAt(buf, 0)
	if err != nil {
		return fmt.Errorf("rawth: failed to write file header: %w", err)
	}
	return nil
}

// we need a new page. check if we have some old ones we can reuse first
// if not we just append to the end of the file. storage is cheap right?
func (p *Pager) AllocatePage() (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// reuse old trash
	if p.header.FreeListHead != 0 {
		pageID := p.header.FreeListHead

		data, err := p.readPageLocked(pageID)
		if err != nil {
			return 0, err
		}

		nextFree := binary.LittleEndian.Uint32(data[0:4])
		p.header.FreeListHead = nextFree

		clear := make([]byte, PageSize)
		p.buffer[pageID] = clear
		p.dirty[pageID] = true

		if err := p.writeHeader(); err != nil {
			return 0, err
		}

		return pageID, nil
	}

	// make new space
	pageID := p.header.PageCount
	p.header.PageCount++

	empty := make([]byte, PageSize)
	offset := int64(pageID) * int64(PageSize)
	if _, err := p.file.WriteAt(empty, offset); err != nil {
		return 0, fmt.Errorf("rawth: failed to allocate page %d: %w", pageID, err)
	}

	p.buffer[pageID] = empty
	p.dirty[pageID] = true

	if err := p.writeHeader(); err != nil {
		return 0, err
	}

	return pageID, nil
}

// put a page into the recycling bin. we dont actually shrink the file
// because that is hard and i dont want to deal with it today
func (p *Pager) FreePage(pageID uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if pageID == 0 {
		return errors.New("rawth: cannot free the header page, nice try")
	}

	data := make([]byte, PageSize)
	binary.LittleEndian.PutUint32(data[0:4], p.header.FreeListHead)

	p.buffer[pageID] = data
	p.dirty[pageID] = true
	p.header.FreeListHead = pageID

	return p.writeHeader()
}

// give me the bytes for this id. checking cache first because i am efficient
func (p *Pager) ReadPage(pageID uint32) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.readPageLocked(pageID)
}

func (p *Pager) readPageLocked(pageID uint32) ([]byte, error) {
	if pageID >= p.header.PageCount {
		return nil, fmt.Errorf("rawth: page %d out of range (total: %d)", pageID, p.header.PageCount)
	}

	if data, ok := p.buffer[pageID]; ok {
		cp := make([]byte, PageSize)
		copy(cp, data)
		return cp, nil
	}

	// actual disk work ugh
	data := make([]byte, PageSize)
	offset := int64(pageID) * int64(PageSize)
	_, err := p.file.ReadAt(data, offset)
	if err != nil {
		return nil, fmt.Errorf("rawth: failed to read page %d: %w", pageID, err)
	}

	if len(p.buffer) < MaxBufferPages {
		cp := make([]byte, PageSize)
		copy(cp, data)
		p.buffer[pageID] = cp
	}

	return data, nil
}

// write to the buffer. it stays "dirty" until we flush it. 
// like my dishes but i eventually flush these.
func (p *Pager) WritePage(pageID uint32, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(data) != PageSize {
		return fmt.Errorf("rawth: page data must be exactly %d bytes, got %d", PageSize, len(data))
	}

	cp := make([]byte, PageSize)
	copy(cp, data)
	p.buffer[pageID] = cp
	p.dirty[pageID] = true

	return nil
}

// dump everything from memory to the actual file. 
// if the power goes out before this we lose data but thats life.
func (p *Pager) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for pageID := range p.dirty {
		data, ok := p.buffer[pageID]
		if !ok {
			continue
		}
		offset := int64(pageID) * int64(PageSize)
		if _, err := p.file.WriteAt(data, offset); err != nil {
			return fmt.Errorf("rawth: failed to flush page %d: %w", pageID, err)
		}
	}

	p.dirty = make(map[uint32]bool)

	return p.file.Sync()
}

func (p *Pager) SetRootPage(pageID uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.header.RootPageID = pageID
	return p.writeHeader()
}

func (p *Pager) RootPage() uint32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.header.RootPageID
}

func (p *Pager) PageCount() uint32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.header.PageCount
}

func (p *Pager) FileSize() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return int64(p.header.PageCount) * int64(PageSize)
}

func (p *Pager) Close() error {
	if err := p.Flush(); err != nil {
		p.file.Close()
		return err
	}
	return p.file.Close()
}
