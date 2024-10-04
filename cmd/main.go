package main

import (
	"fmt"
	"log"
	"net/http"

	"ethparser/internal/parser"
)

type httpHandler struct {
	parser parser.Parser
}

func main() {
	parser, err := parser.NewEthParser()
	if err != nil {
		log.Fatal(err)
	}

	handler := &httpHandler{parser: parser}

	http.HandleFunc("/transactions", handler.handleGetTransactions)
	http.HandleFunc("/subscribe", handler.handleSubscribe)
	http.HandleFunc("/currentBlock", handler.handleGetCurrentBlock)

	fmt.Println("Starting server on 9090")
	if err := http.ListenAndServe(":9090", nil); err != nil {
		log.Fatal(err)
	}
}

func (hh *httpHandler) handleGetTransactions(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	transactions := hh.parser.GetTransactions(address)
	w.WriteHeader(http.StatusOK)

	for _, tx := range transactions {
		w.Write([]byte(fmt.Sprintf("%v", tx.Hash)))
	}
}

func (hh *httpHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res := hh.parser.Subscribe(address)
	if !res {
		http.Error(w, "failed to subscribe", http.StatusInternalServerError)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("subscribed"))
}

func (hh *httpHandler) handleGetCurrentBlock(w http.ResponseWriter, r *http.Request) {
	int := hh.parser.GetCurrentBlock()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("%v", int)))
}
