package p2p

import (
	"bufio"
	"fmt"
	"log"
	"time"
	"strings"
	"github.com/elon0823/paust-db/types"
	"github.com/golang/protobuf/proto"
	host "github.com/libp2p/go-libp2p-host"
	net "github.com/libp2p/go-libp2p-net"
)

type BootstrapNode struct {
	Address   string
	Port      string
	BasicHost host.Host
	Secio     bool
	Randseed  int64
	SuperNodePool NodePool
}

func NewBootstrapNode(address string, listenPort string, secio bool, randseed int64) (*BootstrapNode, error) {

	host, _ := makeBasicHost(address, listenPort, secio, randseed)

	return &BootstrapNode{
		Address:   address,
		Port:      listenPort,
		BasicHost: host,
		Secio:     secio,
		Randseed:  randseed,
		SuperNodePool:   NodePool{
			TimeoutSec: 60,
		},
	}, nil
}

func (bootstrapNode *BootstrapNode) Run() {
	log.Println("running bootstrap node..")
	log.Println("listening for connections")
	// Set a stream handler on host A. /p2p/1.0.0 is
	// a user-defined protocol name.
	bootstrapNode.BasicHost.SetStreamHandler("/p2p/bootstrap/1.0.0", bootstrapNode.handleStream)
	// go p2pManager.writeData()
	
	ticker := time.NewTicker(time.Second * 5)
	go func() {
		for range ticker.C {
			bootstrapNode.SuperNodePool.CheckTimeout()
		}
	}()

	select {} // hang forever
}

func (bootstrapNode *BootstrapNode) handleStream(s net.Stream) {

	//p2pManager.registerStream(s)

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	go bootstrapNode.readData(rw, s)

	log.Println("Got a new stream!")
}

func (bootstrapNode *BootstrapNode) readData(rw *bufio.ReadWriter, s net.Stream) {

	for {
		receivedStr, err := rw.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from buffer")
			rw = nil
			s.Close()
			break
		}

		if receivedStr == "" {
			return
		}
		if receivedStr != "\n" {
			// Green console colour: 	\x1b[32m
			// Reset console colour: 	\x1b[0m
			log.Println("received from peer ", s.Conn().RemotePeer().String())

			str := strings.Replace(receivedStr, "\n", "", -1)
			str = strings.Replace(str, "|bbaa", "\n", -1)

			p2pMessage := &types.P2PMessage{}
			if err := proto.Unmarshal([]byte(str), p2pMessage); err != nil {
				log.Fatal(err)
			} else {
				log.Println("original sender peer ", p2pMessage.Sender)

				switch p2pMessage.Path {
				case types.MSG:
					fmt.Println(string(p2pMessage.Data))
				case types.PUB_SUPERNODE:
					nodeHeader := &NodeHeader{}
					if err := proto.Unmarshal([]byte(p2pMessage.Data), nodeHeader); err != nil {
						log.Fatal(err)
					} else {
						nodePulse := &NodePulse {
							NodeHeader: *nodeHeader,
							LastTimestamp: time.Now().Unix(),
						}
						bootstrapNode.SuperNodePool.AddNodePulse(*nodePulse)
						fmt.Println("super node registed with peer id = ", nodeHeader.PeerId)
					}
				case types.HEARTBEAT:
					nodeHeader := &NodeHeader{}
					if err := proto.Unmarshal([]byte(p2pMessage.Data), nodeHeader); err != nil {
						log.Fatal(err)
					} else {
						bootstrapNode.SuperNodePool.Update(nodeHeader.PeerId)
						fmt.Println("heartbeat with peer id = ", nodeHeader.PeerId)
					}
				default:
					fmt.Println("no method ", p2pMessage.Path)
				}
			}
		}
	}
}
