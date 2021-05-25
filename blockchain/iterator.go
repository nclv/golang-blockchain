package blockchain

import (
	"log"

	"github.com/dgraph-io/badger"
)

type Iterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func (chain *BlockChain) Iterator() *Iterator {
	iter := &Iterator{chain.LastHash, chain.Database}

	return iter
}

func (iter *Iterator) Next() *Block {
	var block *Block

	if err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		if err != nil {
			log.Panic(err)
		}

		encodedBlock, err := item.ValueCopy(nil)
		block = DeserializeBlock(encodedBlock)

		return err
	}); err != nil {
		log.Panic(err)
	}

	iter.CurrentHash = block.PrevHash

	return block
}
