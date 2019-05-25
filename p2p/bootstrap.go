package p2p

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/elon0823/paust-db/types"
	"github.com/golang/protobuf/proto"
	host "github.com/libp2p/go-libp2p-host"
	net "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

type BootstrapNode struct {
	Address   string
	Port      string
	BasicHost host.Host
	Secio     bool
	Randseed  int64
}

func NewBootstrapNode(address string, listenPort string, secio bool, randseed int64) (*BootstrapNode, error) {

	host, _ := makeBasicHost(address, listenPort, secio, randseed)

	return &BootstrapNode{
		Address:   address,
		Port:      listenPort,
		BasicHost: host,
		Secio:     secio,
		Randseed:  randseed,
	}, nil
}

func (bootstrapNode *BootstrapNode) Run(target string) {

	if target == "" {
		log.Println("listening for connections")
		// Set a stream handler on host A. /p2p/1.0.0 is
		// a user-defined protocol name.
		bootstrapNode.BasicHost.SetStreamHandler("/p2p/1.0.0", bootstrapNode.handleStream)
		// go p2pManager.writeData()

		select {} // hang forever
		/**** This is where the listener code ends ****/
	} else {
		bootstrapNode.BasicHost.SetStreamHandler("/p2p/1.0.0", bootstrapNode.handleStream)

		// The following code extracts target's peer ID from the
		// given multiaddress
		ipfsaddr, err := ma.NewMultiaddr(target)
		if err != nil {
			log.Fatalln(err)
		}

		pid, err := ipfsaddr.ValueForProtocol(ma.P_IPFS)
		if err != nil {
			log.Fatalln(err)
		}

		peerid, err := peer.IDB58Decode(pid)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Println("i m peer ", bootstrapNode.BasicHost.ID())

		// Decapsulate the /ipfs/<peerID> part from the target
		// /ip4/<a.b.c.d>/ipfs/<peer> becomes /ip4/<a.b.c.d>
		targetPeerAddr, _ := ma.NewMultiaddr(
			fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(peerid)))
		targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

		// We have a peer ID and a targetAddr so we add it to the peerstore
		// so LibP2P knows how to contact it

		bootstrapNode.BasicHost.Peerstore().AddAddr(peerid, targetAddr, pstore.PermanentAddrTTL)

		log.Println("opening stream")
		// make a new stream from host B to host A
		// it should be handled on host A by the handler we set above because
		// we use the same /p2p/1.0.0 protocol
		s, err := bootstrapNode.BasicHost.NewStream(context.Background(), peerid, "/p2p/1.0.0")

		if err != nil {
			log.Fatalln(err)
		}

		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

		go bootstrapNode.readData(rw, s)

		select {} // hang forever

	}
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
			panic(err)
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
					fmt.Println("super node registered with peer id ", s.Conn().RemotePeer().String())
				default:
					fmt.Println("no method ", p2pMessage.Path)
				}
			}
		}
	}
}
