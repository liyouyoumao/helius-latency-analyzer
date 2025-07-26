package main

import (
	"sync"
	"sync/atomic"

	"github.com/blocto/solana-go-sdk/client"
)

// BlockCache uses a ring buffer for O(1) operations and reduced memory allocation
type BlockCache struct {
	capacity int
	mu       sync.RWMutex
	blocks   []*blockWrapper
	head     int32
	tail     int32
	size     int32
}

type blockWrapper struct {
	slot  int64
	block *client.Block
}

func NewBlockCache(capacity int) *BlockCache {
	return &BlockCache{
		capacity: capacity,
		blocks:   make([]*blockWrapper, capacity),
	}
}

func (bc *BlockCache) AddBlock(slot int64, block *client.Block) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Check if block already exists using binary search
	left := 0
	right := int(atomic.LoadInt32(&bc.size)) - 1

	for left <= right {
		mid := (left + right) / 2
		idx := (int(atomic.LoadInt32(&bc.tail)) + mid) % bc.capacity

		if bc.blocks[idx] == nil {
			break
		}

		if bc.blocks[idx].slot == slot {
			return // Block already exists
		}

		if bc.blocks[idx].slot < slot {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	// Add new block
	head := int(atomic.LoadInt32(&bc.head))
	bc.blocks[head] = &blockWrapper{slot: slot, block: block}

	// Update head atomically
	newHead := (head + 1) % bc.capacity
	atomic.StoreInt32(&bc.head, int32(newHead))

	// Update size and tail if needed
	if atomic.LoadInt32(&bc.size) < int32(bc.capacity) {
		atomic.AddInt32(&bc.size, 1)
	} else {
		atomic.StoreInt32(&bc.tail, (atomic.LoadInt32(&bc.tail)+1)%int32(bc.capacity))
	}
}

func (bc *BlockCache) GetBlock(slot int64) *client.Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Binary search for the block
	left := 0
	right := int(atomic.LoadInt32(&bc.size)) - 1

	for left <= right {
		mid := (left + right) / 2
		idx := (int(atomic.LoadInt32(&bc.tail)) + mid) % bc.capacity

		if bc.blocks[idx] == nil {
			break
		}

		if bc.blocks[idx].slot == slot {
			return bc.blocks[idx].block
		}

		if bc.blocks[idx].slot < slot {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	return nil
}
