package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger"
)

const (
	dbPath      = "./tmp/blocks_%s"
	genesisData = "First block data"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

func DBexists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}

	return true
}

func Retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}

	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)

	return db, err
}

func OpenDB(dir string, opts badger.Options) (*badger.DB, error) {
	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := Retry(dir, opts); err != nil {
				log.Println("Database unlocked, value log truncated")
				return db, nil
			}
		}
		return nil, err
	} else {
		return db, nil
	}
}

func InitBlockChain(address, nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	if DBexists(path) {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	db, err := OpenDB(path, badger.DefaultOptions(path))
	if err != nil {
		log.Panic(err)
	}

	var lastHash []byte
	if err := db.Update(func(txn *badger.Txn) error {
		cbtx := CoinbaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis created")

		if err = txn.Set(genesis.Hash, genesis.Serialize()); err != nil {
			log.Panic(err)
		}
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	}); err != nil {
		log.Panic(err)
	}

	blockchain := BlockChain{lastHash, db}

	return &blockchain
}

func ContinueBlockChain(nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	if DBexists(path) == false {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	db, err := OpenDB(path, badger.DefaultOptions(path))
	if err != nil {
		log.Panic(err)
	}

	var lastHash []byte
	if err := db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, err = item.ValueCopy(nil)

		return err
	}); err != nil {
		log.Panic(err)
	}

	blockchain := BlockChain{lastHash, db}

	return &blockchain
}

func (chain *BlockChain) AddBlock(block *Block) {
	if err := chain.Database.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}
		blockData := block.Serialize()
		err := txn.Set(block.Hash, blockData)
		if err != nil {
			log.Panic(err)
		}

		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		if err != nil {
			log.Panic(err)
		}
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock := DeserializeBlock(lastBlockData)

		if block.Height > lastBlock.Height {
			if err := txn.Set([]byte("lh"), block.Hash); err != nil {
				log.Panic(err)
			}
			chain.LastHash = block.Hash
		}

		return nil
	}); err != nil {
		log.Panic(err)
	}
}

func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	if err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("block is not found")
		} else {
			blockData, _ := item.ValueCopy(nil)

			block = *DeserializeBlock(blockData)
		}

		return nil
	}); err != nil {
		return block, err
	}

	return block, nil
}

func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block := iter.Next()

		blocks = append(blocks, block.Hash)

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block

	if err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		if err != nil {
			log.Panic(err)
		}
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock = *DeserializeBlock(lastBlockData)

		return nil
	}); err != nil {
		log.Panic(err)
	}

	return lastBlock.Height
}

func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block {
	var lastHash []byte
	var lastHeight int

	if err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, err = item.ValueCopy(nil)
		if err != nil {
			log.Panic(err)
		}
		item, err = txn.Get(lastHash)
		if err != nil {
			log.Panic(err)
		}
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock := DeserializeBlock(lastBlockData)

		lastHeight = lastBlock.Height

		return err
	}); err != nil {
		log.Panic(err)
	}

	newBlock := CreateBlock(transactions, lastHash, lastHeight+1)

	if err := chain.Database.Update(func(txn *badger.Txn) error {
		if err := txn.Set(newBlock.Hash, newBlock.Serialize()); err != nil {
			log.Panic(err)
		}
		err := txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	}); err != nil {
		log.Panic(err)
	}

	return newBlock
}

func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spendTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spendTXOs[txID] != nil {
					for _, spentOut := range spendTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}

			if tx.IsCoinbase() == false {
				// find other outputs that are referenced by inputs
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spendTXOs[inTxID] = append(spendTXOs[inTxID], in.Out)
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return UTXO
}

func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("transaction does not exist")
}

func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		if err != nil {
			log.Panic(err)
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		if err != nil {
			log.Panic(err)
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}

func DeserializeTransaction(data []byte) Transaction {
	var transaction Transaction

	decoder := gob.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&transaction); err != nil {
		log.Panic(err)
	}

	return transaction
}
