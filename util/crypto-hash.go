package util

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"github.com/elon0823/paust-db/types"
	"github.com/golang/protobuf/proto"
	"time"
)
func CalculateHash(inputs ...string) string {

	record := ""
	for _, item := range inputs {
		record += item
	}
	
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func MakeMsgBytes(path int32, data []byte, sender string) ([]byte) {

	p2pMsg := &types.P2PMessage{
		Id:        "",
		Path:      path,
		Data:      data,
		Timestamp: uint64(time.Now().UnixNano()),
		Sender:    sender,
	}
	
	p2pMsg.Id = CalculateHash(strconv.FormatUint(p2pMsg.Timestamp, 10), strconv.FormatInt(int64(p2pMsg.Path), 10), string(p2pMsg.Data))
	bytes, _ := proto.Marshal(p2pMsg)

	return bytes

}