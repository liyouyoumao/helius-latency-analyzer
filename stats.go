package main

import (
	"sync"
	"time"
)

// Time 存储区块的时间信息
type Time struct {
	BlockTime  int64 // Block timestamp in milliseconds
	LocalTime  int64 // Local receive time in milliseconds
	NetworkLat int64 // Network latency in milliseconds
	ProcessLat int64 // Processing latency in milliseconds
}

// SyncStats 统计数据结构
type SyncStats struct {
	mu      sync.Mutex
	history []Time
	length  int
}

func NewSyncStats(maxHistory int) *SyncStats {
	return &SyncStats{
		history: make([]Time, 0, maxHistory),
		length:  maxHistory,
	}
}

// AddBlockTime 记录区块信息
func (b *SyncStats) AddBlockTime(blockTimestamp int64, networkLat, processLat int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().UnixMilli()        // Local time in milliseconds
	blockTimeMs := blockTimestamp * 1000 // Convert block time to milliseconds

	b.history = append(b.history, Time{
		BlockTime:  blockTimeMs,
		LocalTime:  now,
		NetworkLat: networkLat,
		ProcessLat: processLat,
	})

	// Limit history length
	if len(b.history) > b.length {
		b.history = b.history[1:]
	}
}

// GetStats 计算时间差和网络波动
func (b *SyncStats) GetStats() (blockInterval int64, avgTotalLat float64, avgNetworkLat float64, avgProcessLat float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(b.history)
	if n < 2 {
		return 0, 0, 0, 0
	}

	// Calculate block interval using local receive times
	// This gives us the actual time between receiving blocks
	blockInterval = b.history[n-1].LocalTime - b.history[n-2].LocalTime

	// Calculate averages
	var networkSum, processSum, totalSum int64
	for _, block := range b.history {
		networkSum += block.NetworkLat
		processSum += block.ProcessLat
		totalSum += block.LocalTime - block.BlockTime
	}

	avgNetworkLat = float64(networkSum) / float64(n)
	avgProcessLat = float64(processSum) / float64(n)
	avgTotalLat = float64(totalSum) / float64(n)

	// Calculate standard deviations
	// var networkSquaredSum, processSquaredSum, totalSquaredSum float64
	// for _, block := range b.history {
	// 	totalLat := block.NetworkLat + block.ProcessLat
	// 	networkSquaredSum += math.Pow(float64(block.NetworkLat)-avgNetworkLat, 2)
	// 	processSquaredSum += math.Pow(float64(block.ProcessLat)-avgProcessLat, 2)
	// 	totalSquaredSum += math.Pow(float64(totalLat)-avgTotalLat, 2)
	// }

	// networkStdDev = math.Sqrt(networkSquaredSum / float64(n))
	// processStdDev = math.Sqrt(processSquaredSum / float64(n))
	// totalLatStdDev = math.Sqrt(totalSquaredSum / float64(n))

	return blockInterval, avgTotalLat, avgNetworkLat, avgProcessLat
}
