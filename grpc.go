package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"solgrpc/proto"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/klauspost/compress/zstd"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

const (
	zstdName = "zstd"
	// Performance tuning constants
	initialWindowSize     = 16 * 1024 * 1024  // 16MB
	initialConnWindowSize = 32 * 1024 * 1024  // 32MB
	maxRecvMsgSize        = 512 * 1024 * 1024 // 512MB
	maxSendMsgSize        = 512 * 1024 * 1024 // 512MB
	batchSize             = 128               // Increased batch size
	prefetchSize          = 256               // Increased prefetch size
	workerMultiplier      = 8                 // Increased worker multiplier
)

// zstdCompressor implements the encoding.Compressor interface
type zstdCompressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

func newZstdCompressor() *zstdCompressor {
	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	decoder, _ := zstd.NewReader(nil)
	return &zstdCompressor{
		encoder: encoder,
		decoder: decoder,
	}
}

func (c *zstdCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedFastest))
}

func (c *zstdCompressor) Decompress(r io.Reader) (io.Reader, error) {
	return zstd.NewReader(r)
}

func (c *zstdCompressor) Name() string {
	return zstdName
}

func init() {
	encoding.RegisterCompressor(newZstdCompressor())
}

var kacp = keepalive.ClientParameters{
	Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
	Timeout:             3 * time.Second,  // wait 1 second for ping ack before considering the connection dead
	PermitWithoutStream: true,             // send pings even without active streams
}

// BlockProcessor handles parallel block processing
type BlockProcessor struct {
	workers    int
	queue      chan *proto.SubscribeUpdateBlock
	wg         sync.WaitGroup
	cache      *BlockCache
	batch      []*proto.SubscribeUpdateBlock
	batchMu    sync.Mutex
	prefetch   chan *proto.SubscribeUpdateBlock
	prefetchWg sync.WaitGroup
	processed  uint64
	blockPool  sync.Pool
	processCh  chan struct{} // Channel to signal processing completion
}

func NewBlockProcessor(workers int, cache *BlockCache) *BlockProcessor {
	if workers <= 0 {
		workers = runtime.NumCPU() * workerMultiplier
	}
	bp := &BlockProcessor{
		workers:   workers,
		queue:     make(chan *proto.SubscribeUpdateBlock, workers*16),
		cache:     cache,
		batch:     make([]*proto.SubscribeUpdateBlock, 0, batchSize),
		prefetch:  make(chan *proto.SubscribeUpdateBlock, prefetchSize),
		processCh: make(chan struct{}, 1),
		blockPool: sync.Pool{
			New: func() interface{} {
				return &client.Block{}
			},
		},
	}

	// Start prefetch workers
	for i := 0; i < workers/2; i++ {
		bp.wg.Add(1)
		go func() {
			defer bp.wg.Done()
			for block := range bp.prefetch {
				converted := bp.blockPool.Get().(*client.Block)
				err := convertBlockFast(block, converted)
				if err != nil {
					logrus.Errorf("failed to convert block: %v", err)
					bp.blockPool.Put(converted)
					continue
				}
				bp.cache.AddBlock(int64(block.GetSlot()), converted)
				atomic.AddUint64(&bp.processed, 1)
				bp.processCh <- struct{}{}
			}
		}()
	}

	return bp
}

func (bp *BlockProcessor) Process(block *proto.SubscribeUpdateBlock) {
	// Process block immediately
	converted := bp.blockPool.Get().(*client.Block)
	err := convertBlockFast(block, converted)
	if err != nil {
		logrus.Errorf("failed to convert block: %v", err)
		bp.blockPool.Put(converted)
		return
	}
	bp.cache.AddBlock(int64(block.GetSlot()), converted)
	atomic.AddUint64(&bp.processed, 1)
	bp.processCh <- struct{}{}
}

func (bp *BlockProcessor) Start() {
	// No need to start workers since we're processing immediately
}

func (bp *BlockProcessor) Stop() {
	close(bp.queue)
	close(bp.prefetch)
	bp.wg.Wait()
}

// convertBlockFast is an optimized version of convertBlock that reuses the target block
func convertBlockFast(block *proto.SubscribeUpdateBlock, target *client.Block) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}

	// Convert block time
	var blockTime *time.Time
	if block.BlockTime != nil {
		t := time.Unix(block.BlockTime.Timestamp, 0)
		blockTime = &t
	}

	// Convert height
	height := int64(block.Slot)

	// Get transaction slice from pool
	txs := make([]client.BlockTransaction, 0, len(block.Transactions))

	// Process transactions
	for _, vtx := range block.Transactions {
		transactionMeta, err := convertTransactionMeta(vtx.Meta)
		if err != nil {
			return fmt.Errorf("failed to convert transaction meta: %v", err)
		}

		tx, accountKeys, err := parseTx(vtx.Transaction, transactionMeta)
		if err != nil {
			return fmt.Errorf("failed to parse tx: %v", err)
		}

		txs = append(txs, client.BlockTransaction{
			Meta:        transactionMeta,
			Transaction: tx,
			AccountKeys: accountKeys,
		})
	}

	// Update target block
	target.ParentSlot = block.ParentSlot
	target.PreviousBlockhash = block.ParentBlockhash
	target.Blockhash = block.Blockhash
	target.BlockHeight = &height
	target.BlockTime = blockTime
	target.Transactions = txs

	return nil
}

type GrpcClient struct {
	name      string
	cache     *BlockCache
	length    int
	mux       sync.RWMutex
	endpoint  string
	xToken    string
	conn      *grpc.ClientConn
	client    proto.GeyserClient
	processor *BlockProcessor
}

func NewGrpcClient(name, endpoint, xToken string, length int) (*GrpcClient, error) {
	cache := NewBlockCache(length)
	gm := &GrpcClient{
		name:      name,
		endpoint:  endpoint,
		xToken:    xToken,
		cache:     cache,
		processor: NewBlockProcessor(runtime.NumCPU()*workerMultiplier, cache),
	}

	// Establish connection immediately
	if err := gm.connect(); err != nil {
		return nil, err
	}

	gm.processor.Start()
	return gm, nil
}

func (c *GrpcClient) connect() error {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return fmt.Errorf("invalid GRPC address provided: %v", err)
	}

	var insecureConnection bool
	if u.Scheme == "http" {
		insecureConnection = true
	}

	port := u.Port()
	if port == "" {
		if insecureConnection {
			port = "80"
		} else {
			port = "443"
		}
	}
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("please provide URL format endpoint e.g. http(s)://<endpoint>:<port>")
	}

	address := hostname + ":" + port

	var opts []grpc.DialOption
	if insecureConnection {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		pool, _ := x509.SystemCertPool()
		creds := credentials.NewClientTLSFromCert(pool, "")
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	// Optimize gRPC options
	opts = append(opts, grpc.WithKeepaliveParams(kacp))
	opts = append(opts, grpc.WithDefaultCallOptions(
		grpc.MaxCallSendMsgSize(maxSendMsgSize),
		grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
	))
	opts = append(opts, grpc.WithInitialWindowSize(initialWindowSize))
	opts = append(opts, grpc.WithInitialConnWindowSize(initialConnWindowSize))
	opts = append(opts, grpc.WithNoProxy())
	opts = append(opts, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	opts = append(opts, grpc.WithDefaultCallOptions(grpc.UseCompressor(zstdName)))

	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return err
	}

	c.conn = conn
	c.client = proto.NewGeyserClient(conn)
	return nil
}

func (c *GrpcClient) GetBlock(height int64) *client.Block {
	return c.cache.GetBlock(height)
}

func (c *GrpcClient) SubscribeBlock() {
	stats := NewSyncStats(100)
	//logrus.Info("Starting block subscription...")

	// Create a basic subscription request
	var True = true
	subscription := proto.SubscribeRequest{
		Blocks: map[string]*proto.SubscribeRequestFilterBlocks{
			"blocks": {
				IncludeTransactions: &True,
			},
		},
		Commitment: proto.CommitmentLevel_PROCESSED.Enum(),
	}

	md := metadata.New(map[string]string{
		"x-token": c.xToken,
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	for {
		if c.client == nil {
			if err := c.connect(); err != nil {
				logrus.Errorf("error connecting to grpc endpoint: %v", err)
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		stream, err := c.client.Subscribe(ctx)
		if err != nil {
			logrus.Errorf("failed to subscribe: %s", err.Error())
			c.client = nil
			continue
		}

		err = stream.Send(&subscription)
		if err != nil {
			logrus.Errorf("failed to send subscription request: %v", err)
			c.client = nil
			continue
		}

		for {
			receiveStart := time.Now()
			resp, err := stream.Recv()
			if err != nil {
				logrus.Errorf("Error occurred in receiving update: %v", err)
				c.client = nil
				break
			}

			if resp == nil || resp.GetBlock() == nil {
				continue
			}

			networkLat := time.Since(receiveStart).Milliseconds()
			processStart := time.Now()

			// Process block using the worker pool
			c.processor.Process(resp.GetBlock())

			// Wait for processing to complete
			<-c.processor.processCh
			processLat := time.Since(processStart).Milliseconds()
			//totalLat := networkLat + processLat

			// Convert block time to milliseconds for consistency
			blockTimeMs := resp.GetBlock().BlockTime.Timestamp

			stats.AddBlockTime(blockTimeMs, networkLat, processLat)

			_, avgTotalLat, avgNetworkLat, _ := stats.GetStats()
			logrus.WithFields(logrus.Fields{
				"AvgNetworkLatency": fmt.Sprintf("%.2f", avgNetworkLat),
				"AvgBlockLatency":   fmt.Sprintf("%.2f", avgTotalLat),
				"Block":             resp.GetBlock().Slot,
				"Type":              c.name,
			}).Info("Metrics")
			// logrus.Infof("Type: %s, Block %d Metrics:Time Between Blocks: %4d ms,Avg Block Latency: %.2f ms,Avg Network Latency: %.2f ms",
			// 	c.name,
			// 	resp.GetBlock().Slot,
			// 	blockTimeDiff,
			// 	avgTotalLat,
			// 	avgNetworkLat)
		}
	}
}

func (c *GrpcClient) Close() {
	if c.processor != nil {
		c.processor.Stop()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}
