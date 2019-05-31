package util
import (
	"strings"
	"strconv"
	"github.com/elon0823/paust-db/types"
	"github.com/golang/protobuf/proto"
	"time"
	"fmt"
)

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

func EncapsuleSendMsg(msg string) (string) {

	str := msg
	str = strings.Replace(str, "\n", "|bbaa", -1)

	return fmt.Sprintf("%s\n", str)
}

func DecapsuleReceiveMsg(msg string) (types.P2PMessage, error) {

	str := strings.Replace(msg, "\n", "", -1)
	str = strings.Replace(str, "|bbaa", "\n", -1)

	p2pMessage := &types.P2PMessage{}
	if err := proto.Unmarshal([]byte(str), p2pMessage); err != nil {
		return *p2pMessage, err
	} else {
		return *p2pMessage, nil
	}

}