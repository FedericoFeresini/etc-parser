package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ethparser/internal/cache"
	"ethparser/internal/models"
)

const (
	defaultNodeUrl = "https://cloudflare-eth.com"
)

type Parser interface {
	// GetCurrentBlock gets last parsed block
	GetCurrentBlock() int
	// Subscribe adds address to observer
	Subscribe(address string) bool
	// GetTransactions lists inbound or outbound transactions for an address
	GetTransactions(address string) []*models.Transaction
}

type ethParser struct {
	client *http.Client
	url    string

	m sync.RWMutex
	// addresses is a set of addresses mapped by the latest block number
	// when they were added to the observer
	addresses map[string]int

	transactionCache cache.Cache
}

var _ Parser = &ethParser{}

type JsonRPCRequest struct {
	ID      int           `json:"id"`
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type JsonRPCResponseBlockNumber struct {
	Result string `json:"result"`
}

type JsonRPCResponseBlock struct {
	Result models.BlockWithDetails `json:"result"`
}

type JsonRPCResponseTransaction struct {
	Result models.Transaction `json:"result"`
}

type EthParserOpt func(*ethParser) error

func WithHTTPClient(client *http.Client) EthParserOpt {
	return func(p *ethParser) error {
		if client == nil {
			return errors.New("client cannot be nil")
		}
		p.client = client
		return nil
	}
}

func WithNodeUrl(url string) EthParserOpt {
	return func(p *ethParser) error {
		if url == "" {
			return errors.New("url cannot be empty")
		}
		p.url = url
		return nil
	}
}

func NewEthParser(opts ...EthParserOpt) (*ethParser, error) {
	e := &ethParser{
		url:              defaultNodeUrl,
		client:           http.DefaultClient,
		m:                sync.RWMutex{},
		addresses:        make(map[string]int),
		transactionCache: cache.NewMemCache(),
	}

	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}

	return e, nil
}

func (e *ethParser) GetCurrentBlock() int {
	blockNumber, err := e.getCurrentBlockNumber()
	if err != nil {
		log.Println(err)
		return 0
	}

	return blockNumber
}

func (e *ethParser) Subscribe(address string) bool {
	e.m.Lock()
	defer e.m.Unlock()

	if _, ok := e.addresses[address]; ok {
		log.Println("address already subscribed", address)
		return false
	}

	blockNumber, err := e.getCurrentBlockNumber()
	if err != nil {
		log.Println(err)
		return false
	}

	e.addresses[address] = blockNumber
	return true
}

func (e *ethParser) GetTransactions(address string) []*models.Transaction {
	e.m.RLock()
	defer e.m.RUnlock()

	initialBlockNumber, err := e.getAddressInitialBlockNumber(address)
	if err != nil {
		log.Println(err)
		return nil
	}

	cachedTransactions, cachedBlockNumber := e.transactionCache.GetTransactions(address)

	currentBlockNumber := e.GetCurrentBlock()
	if cachedBlockNumber == currentBlockNumber {
		return cachedTransactions
	}

	var fromBlockNumber int
	var toBlockNumber int

	if cachedBlockNumber == 0 {
		fromBlockNumber = initialBlockNumber
		toBlockNumber = currentBlockNumber
	} else {
		fromBlockNumber = cachedBlockNumber
		toBlockNumber = currentBlockNumber
	}

	transactions, err := e.getTransactionsFromBlockNumbers(fromBlockNumber, toBlockNumber, address)
	if err != nil {
		log.Println(err)
		return nil
	}

	if len(cachedTransactions) > 0 {
		transactions = append(transactions, cachedTransactions...)
	}

	e.transactionCache.AddTransactions(address, transactions, toBlockNumber)
	return transactions
}

// getAddressInitialBlockNumber gets the initial block number for an address
func (e *ethParser) getAddressInitialBlockNumber(address string) (int, error) {
	e.m.RLock()
	defer e.m.RUnlock()

	blockNumber, ok := e.addresses[address]
	if !ok {
		return 0, fmt.Errorf("address not found in the observer: %s", address)
	}

	return blockNumber, nil
}

// getCurrentBlockNumber gets the current block number
func (e *ethParser) getCurrentBlockNumber() (int, error) {
	rpcRequest := JsonRPCRequest{
		ID:      1,
		Jsonrpc: "2.0",
		Method:  "eth_blockNumber",
		Params:  []interface{}{},
	}

	rpcResponse, err := do[JsonRPCResponseBlockNumber](rpcRequest, e.url)
	if err != nil {
		return 0, err
	}

	blockNumber, err := strconv.ParseInt(rpcResponse.Result, 0, 0)
	if err != nil {
		log.Println(err)
		return 0, err
	}

	return int(blockNumber), nil
}

// getTransactionsFromBlockNumber gets transactions from startBlock to endBlock
func (e *ethParser) getTransactionsFromBlockNumbers(endingBlockNumber, headBlockNumber int, address string) ([]*models.Transaction, error) {
	var allTransactions []*models.Transaction

	req := JsonRPCRequest{
		ID:      1,
		Jsonrpc: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []interface{}{intToHex(headBlockNumber), true},
	}

	rpcResponse, err := do[JsonRPCResponseBlock](req, e.url)
	if err != nil {
		return nil, err
	}

	log.Println("fetching transactions for block", headBlockNumber)

	transactions, err := e.getTransactionsFromBlock(&rpcResponse.Result, address)
	if err != nil {
		return nil, err
	}

	allTransactions = append(allTransactions, transactions...)

	if headBlockNumber == endingBlockNumber {
		return allTransactions, nil
	}

	transactions, err = e.getTransactionsInBlockRange(endingBlockNumber, rpcResponse.Result.ParentHash, address)
	if err != nil {
		return nil, err
	}

	allTransactions = append(allTransactions, transactions...)

	return allTransactions, nil
}

// getTransactionsFromBlockHash recursively gets transactions from blocks
// moving from headBlockHash to the lastBlockNumber
func (e *ethParser) getTransactionsInBlockRange(endingBlockNumber int, headBlockHash string, address string) ([]*models.Transaction, error) {
	var allTransactions []*models.Transaction

	req := JsonRPCRequest{
		ID:      1,
		Jsonrpc: "2.0",
		Method:  "eth_getBlockByHash",
		Params:  []interface{}{headBlockHash, true},
	}

	var rpcResponse *JsonRPCResponseBlock
	var err error

	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(i) * time.Second)
		rpcResponse, err = do[JsonRPCResponseBlock](req, e.url)
		if err == nil && rpcResponse.Result.Number != "" {
			break
		}
	}

	log.Println("fetching transactions for block", rpcResponse.Result.Number)

	if err != nil {
		return nil, err
	}

	transactions, err := e.getTransactionsFromBlock(&rpcResponse.Result, address)
	if err != nil {
		return nil, err
	}
	allTransactions = append(allTransactions, transactions...)

	blockNumber, err := strconv.ParseInt(rpcResponse.Result.Number, 0, 0)
	if err != nil {
		return nil, err
	}

	if int(blockNumber) == endingBlockNumber {
		return allTransactions, nil
	}

	transactions, err = e.getTransactionsInBlockRange(endingBlockNumber, rpcResponse.Result.ParentHash, address)
	if err != nil {
		return nil, err
	}
	allTransactions = append(allTransactions, transactions...)

	return allTransactions, nil
}

// getBlockFromNumber gets block by block number
func (e *ethParser) getBlockFromNumber(blockNumber int) (*models.BlockWithDetails, error) {
	rpcRequest := JsonRPCRequest{
		ID:      1,
		Jsonrpc: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []interface{}{intToHex(blockNumber), true},
	}

	rpcResponse, err := do[JsonRPCResponseBlock](rpcRequest, e.url)
	if err != nil {
		return nil, err
	}

	return &rpcResponse.Result, nil
}

// getTransactionsFromBlock gets transactions from a block and filters them by address
func (e *ethParser) getTransactionsFromBlock(block *models.BlockWithDetails, address string) ([]*models.Transaction, error) {
	var allTransactions []*models.Transaction
	for _, tx := range block.Transactions {
		if tx.To == address || tx.From == address {
			allTransactions = append(allTransactions, &tx)
		}
	}

	return allTransactions, nil
}

// do sends a JSON RPC request to the node and returns a response
func do[T any](rpcRequest JsonRPCRequest, url string) (*T, error) {
	requestBody, err := json.Marshal(rpcRequest)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResponse T
	err = json.Unmarshal(responseBody, &rpcResponse)
	if err != nil {
		return nil, err
	}

	return &rpcResponse, nil
}

func intToHex(i int) string {
	hexString := strconv.FormatInt(int64(i), 16) // Convert int to int64 and then to hex
	return fmt.Sprintf("0x%s", hexString)
}
