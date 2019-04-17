package blockchain

import (
	db "github.com/elon0823/paust-db/db"
	"os"
	"github.com/pkg/errors"
)

type BlockStore struct {
	DB *db.CRocksDB
}

var bsInstance *BlockStore

func InitBlockStore(storePath string) {
	
	bsInstance, _ = newBlockStore(storePath)
}
func newBlockStore(storePath string) (*BlockStore, error) {
	
	dir := storePath
	dbname := "paustdb"
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, errors.Wrap(err, "make directory failed")
	}
	database, err := db.NewCRocksDB(dbname, dir)
	if err != nil {
		return nil, errors.Wrap(err, "NewCRocksDB err")
	}
	
	return &BlockStore{
		DB: database,
	}, nil
}