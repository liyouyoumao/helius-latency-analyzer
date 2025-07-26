package main

import (
	"fmt"
	"solgrpc/proto"
	"sync"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/btcsuite/btcd/btcutil/base58"
)

// Pre-allocate common slices to reduce allocations
var (
	emptyTime time.Time
	emptyTxs  = make([]client.BlockTransaction, 0, 100)
)

// BlockConverter handles efficient block conversion
type BlockConverter struct {
	txPool sync.Pool
}

func NewBlockConverter() *BlockConverter {
	return &BlockConverter{
		txPool: sync.Pool{
			New: func() interface{} {
				return make([]client.BlockTransaction, 0, 100)
			},
		},
	}
}

func (bc *BlockConverter) convertBlock(v *proto.SubscribeUpdateBlock) (*client.Block, error) {
	if v == nil {
		return nil, nil
	}

	var blockTime *time.Time
	if v.BlockTime != nil {
		t := time.Unix(v.BlockTime.Timestamp, 0)
		blockTime = &t
	}

	height := int64(v.Slot)

	// Get transaction slice from pool
	txs := bc.txPool.Get().([]client.BlockTransaction)
	defer bc.txPool.Put(txs[:0]) // Reset and return to pool

	if len(v.Transactions) > 0 {
		txs = txs[:0] // Reset slice but keep capacity
		for _, vtx := range v.Transactions {
			transactionMeta, err := convertTransactionMeta(vtx.Meta)
			if err != nil {
				return nil, fmt.Errorf("failed to convert transaction meta, err: %v", err)
			}

			tx, accountKeys, err := parseTx(vtx.Transaction, transactionMeta)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tx, err: %v", err)
			}

			txs = append(txs,
				client.BlockTransaction{
					Meta:        transactionMeta,
					Transaction: tx,
					AccountKeys: accountKeys,
				})
		}
	}

	return &client.Block{
		ParentSlot:        v.ParentSlot,
		PreviousBlockhash: v.ParentBlockhash,
		Blockhash:         v.Blockhash,
		BlockHeight:       &height,
		BlockTime:         blockTime,
		Transactions:      txs,
	}, nil
}

// Global converter instance
var converter = NewBlockConverter()

func convertBlock(v *proto.SubscribeUpdateBlock) (*client.Block, error) {
	return converter.convertBlock(v)
}

func parseTx(trx *proto.Transaction, transactionMeta *client.TransactionMeta) (types.Transaction, []common.PublicKey, error) {
	sigs := make([]types.Signature, 0, len(trx.Signatures))
	for _, sig := range trx.Signatures {
		sigs = append(sigs, sig)
	}

	var accountKeys = make([]common.PublicKey, 0, len(trx.Message.AccountKeys))
	for _, accountKey := range trx.Message.AccountKeys {
		accountKeys = append(accountKeys, common.PublicKeyFromBytes(accountKey))
	}

	var ins = make([]types.CompiledInstruction, 0, len(trx.Message.Instructions))
	for _, in := range trx.Message.Instructions {
		var accounts = make([]int, 0, len(in.Accounts))
		for _, account := range in.Accounts {
			accounts = append(accounts, int(account))
		}

		if in.Data == nil {
			in.Data = make([]byte, 0, len(in.Data))
		}

		ins = append(ins, types.CompiledInstruction{
			ProgramIDIndex: int(in.ProgramIdIndex),
			Accounts:       accounts,
			Data:           in.Data,
		})
	}

	var tables = make([]types.CompiledAddressLookupTable, 0, len(trx.Message.AddressTableLookups))
	for _, table := range trx.Message.AddressTableLookups {
		if table.WritableIndexes == nil {
			table.WritableIndexes = make([]uint8, 0, len(table.WritableIndexes))
		}

		if table.ReadonlyIndexes == nil {
			table.ReadonlyIndexes = make([]uint8, 0, len(table.ReadonlyIndexes))
		}

		tables = append(tables, types.CompiledAddressLookupTable{
			AccountKey:      common.PublicKeyFromBytes(table.AccountKey),
			WritableIndexes: table.WritableIndexes,
			ReadonlyIndexes: table.ReadonlyIndexes,
		})
	}
	tx := types.Transaction{
		Signatures: sigs,
		Message: types.Message{
			Version: func() types.MessageVersion {
				if trx.Message.Versioned {
					return "v0"
				}
				return "legacy"
			}(),
			Header: types.MessageHeader{
				NumRequireSignatures:        uint8(trx.Message.Header.NumRequiredSignatures),
				NumReadonlySignedAccounts:   uint8(trx.Message.Header.NumReadonlySignedAccounts),
				NumReadonlyUnsignedAccounts: uint8(trx.Message.Header.NumReadonlyUnsignedAccounts),
			},
			Accounts:            accountKeys,
			RecentBlockHash:     base58.Encode(trx.Message.RecentBlockhash),
			Instructions:        ins,
			AddressLookupTables: tables,
		},
	}
	if transactionMeta != nil {
		for _, s := range transactionMeta.LoadedAddresses.Writable {
			accountKeys = append(accountKeys, common.PublicKeyFromString(s))
		}
		for _, s := range transactionMeta.LoadedAddresses.Readonly {
			accountKeys = append(accountKeys, common.PublicKeyFromString(s))
		}
	}

	return tx, accountKeys, nil
}

func convertReturnData(d *proto.ReturnData) (client.ReturnData, error) {
	programId := common.PublicKeyFromBytes(d.ProgramId)

	return client.ReturnData{
		ProgramId: programId,
		Data:      d.Data,
	}, nil
}

func convertTransactionMeta(meta *proto.TransactionStatusMeta) (*client.TransactionMeta, error) {
	if meta == nil {
		return nil, nil
	}
	innerInstructions := make([]client.InnerInstruction, 0, len(meta.InnerInstructions))
	for _, metaInnerInstruction := range meta.InnerInstructions {
		compiledInstructions := make([]types.CompiledInstruction, 0, len(metaInnerInstruction.Instructions))
		for _, innerInstruction := range metaInnerInstruction.Instructions {
			var accounts = make([]int, 0, len(innerInstruction.Accounts))
			for _, account := range innerInstruction.Accounts {
				accounts = append(accounts, int(account))
			}

			compiledInstructions = append(compiledInstructions,
				types.CompiledInstruction{
					ProgramIDIndex: int(innerInstruction.ProgramIdIndex),
					Data:           innerInstruction.Data,
					Accounts:       accounts,
				})
		}

		innerInstructions = append(innerInstructions, client.InnerInstruction{
			Index:        uint64(metaInnerInstruction.Index),
			Instructions: compiledInstructions,
		})
	}

	var returnData *client.ReturnData
	if v := meta.ReturnData; v != nil {
		d, err := convertReturnData(v)
		if err != nil {
			return nil, fmt.Errorf("failed to process return data, err: %v", err)
		}
		returnData = &d
	}

	var ws = make([]string, 0, len(meta.LoadedWritableAddresses))
	for _, address := range meta.LoadedWritableAddresses {
		ws = append(ws, common.PublicKeyFromBytes(address).String())
	}
	var rs = make([]string, 0, len(meta.LoadedReadonlyAddresses))
	for _, address := range meta.LoadedReadonlyAddresses {
		rs = append(rs, common.PublicKeyFromBytes(address).String())
	}

	var prebalances = make([]int64, 0, len(meta.PreBalances))
	for _, b := range meta.PreBalances {
		prebalances = append(prebalances, int64(b))
	}

	var postbalances = make([]int64, 0, len(meta.PostBalances))
	for _, b := range meta.PostBalances {
		postbalances = append(postbalances, int64(b))
	}

	var pretb = make([]rpc.TransactionMetaTokenBalance, 0, len(meta.PreTokenBalances))
	for _, b := range meta.PreTokenBalances {
		pretb = append(pretb, rpc.TransactionMetaTokenBalance{
			AccountIndex: uint64(b.AccountIndex),
			Mint:         b.Mint,
			Owner:        b.Owner,
			ProgramId:    b.ProgramId,
			UITokenAmount: rpc.TokenAccountBalance{
				Amount: func() string {
					if b.UiTokenAmount.Amount == "" {
						return "0"
					}
					return b.UiTokenAmount.Amount
				}(),
				Decimals:       uint8(b.UiTokenAmount.Decimals),
				UIAmountString: b.UiTokenAmount.UiAmountString,
			},
		})
	}

	var posttb = make([]rpc.TransactionMetaTokenBalance, 0, len(meta.PreTokenBalances))
	for _, b := range meta.PostTokenBalances {
		posttb = append(posttb, rpc.TransactionMetaTokenBalance{
			AccountIndex: uint64(b.AccountIndex),
			Mint:         b.Mint,
			Owner:        b.Owner,
			ProgramId:    b.ProgramId,
			UITokenAmount: rpc.TokenAccountBalance{
				Amount: func() string {
					if b.UiTokenAmount.Amount == "" {
						return "0"
					}
					return b.UiTokenAmount.Amount
				}(),
				Decimals:       uint8(b.UiTokenAmount.Decimals),
				UIAmountString: b.UiTokenAmount.UiAmountString,
			},
		})
	}
	var metaErr any
	if meta.Err == nil {
		metaErr = nil
	} else {
		metaErr = "Failed"
	}

	return &client.TransactionMeta{
		Err:               metaErr,
		Fee:               meta.Fee,
		PreBalances:       prebalances,
		PostBalances:      postbalances,
		PreTokenBalances:  pretb,
		PostTokenBalances: posttb,
		LogMessages:       meta.LogMessages,
		InnerInstructions: innerInstructions,
		LoadedAddresses: rpc.TransactionLoadedAddresses{
			Writable: ws,
			Readonly: rs,
		},
		ReturnData:           returnData,
		ComputeUnitsConsumed: meta.ComputeUnitsConsumed,
	}, nil
}
