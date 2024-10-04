package parser

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	address       = "0xcb81fa1fc2a94461f49d9106dcb7772a29288efe"
	nodeNumberHex = "0x13ecaeb"
)

func TestParserGetCurrentBlock(t *testing.T) {
	parser, err := NewEthParser()
	require.NoError(t, err)

	res := parser.Subscribe(address)
	require.True(t, res)

	blockNumber, err := strconv.ParseInt(nodeNumberHex, 0, 0)
	require.NoError(t, err)

	parser.addresses[address] = int(blockNumber)

	txs := parser.GetTransactions(address)
	require.NotNil(t, txs)

	txs = parser.GetTransactions(address)
	require.NotNil(t, txs)
}
