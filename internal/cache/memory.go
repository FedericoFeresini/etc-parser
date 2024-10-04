package cache

import (
	"sync"

	"ethparser/internal/models"
)

type Cache interface {
	AddTransactions(address string, transactions []*models.Transaction, blockNumber int)
	GetTransactions(address string) ([]*models.Transaction, int)
}

type block struct {
	blockNumber int

	// transactions is a list of transactions by hash
	transactions map[string]*models.Transaction
}

type memCache struct {
	m sync.RWMutex

	// blockTransactions is a map of blocks by addresses
	blockTransactions map[string]block
}

var _ Cache = &memCache{}

func NewMemCache() Cache {
	return &memCache{
		blockTransactions: make(map[string]block),
		m:                 sync.RWMutex{},
	}
}

func (mc *memCache) AddTransactions(address string, transactions []*models.Transaction, blockNumber int) {
	mc.m.Lock()
	defer mc.m.Unlock()

	b, ok := mc.blockTransactions[address]
	if !ok {
		txMap := make(map[string]*models.Transaction)
		for _, tx := range transactions {
			txMap[tx.Hash] = tx
		}

		mc.blockTransactions[address] = block{
			blockNumber:  blockNumber,
			transactions: txMap,
		}
		return
	}

	if b.blockNumber == blockNumber {
		return
	}

	for _, tx := range transactions {
		b.transactions[tx.Hash] = tx
	}

	b.blockNumber = blockNumber
}

func (mc *memCache) GetTransactions(address string) ([]*models.Transaction, int) {
	mc.m.RLock()
	defer mc.m.RUnlock()

	b, ok := mc.blockTransactions[address]
	if !ok {
		return nil, 0
	}

	transactions := make([]*models.Transaction, 0, len(b.transactions))
	for _, tx := range b.transactions {
		transactions = append(transactions, tx)
	}

	return transactions, b.blockNumber
}
