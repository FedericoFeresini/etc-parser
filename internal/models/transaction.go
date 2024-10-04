package models

type Transaction struct {
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	BlockHash   string `json:"blockHash"`
	BlockNumber string `json:"blockNumber"`
}

type BlockWithDetails struct {
	Hash         string        `json:"hash"`
	ParentHash   string        `json:"parentHash"`
	Number       string        `json:"number"`
	Transactions []Transaction `json:"transactions"`
}
