package p2p

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"io"
	"log"
	"strings"
	mrand "math/rand"
	"crypto/rand"
	"github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	net "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"strconv"
	"github.com/golang/protobuf/proto"
	"github.com/elon0823/paust-db/types"
	"time"
	"github.com/elon0823/paust-db/util"
)

type StreamBuffer struct {
	Stream net.Stream
	RW     bufio.ReadWriter
}

type P2PManager struct {
	Address       string
	Port          string
	BasicHost     host.Host
	Secio         bool
	Randseed      int64
	StreamBuffers []StreamBuffer
	HandledMsgBuffer []string
	HandleMsgLimit int
}

func NewP2PManager(address string, listenPort string, secio bool, randseed int64) (*P2PManager, error) {

	host, _ := makeBasicHost(address, listenPort, secio, randseed)
	
	return &P2PManager{
		Address:   address,
		Port:      listenPort,
		BasicHost: host,
		Secio:     secio,
		Randseed:  randseed,
		HandleMsgLimit: 10,
	}, nil
}

func makeBasicHost(address string, listenPort string, secio bool, randseed int64) (host.Host, error) {

	var r io.Reader
	if randseed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(randseed))
	}

	// Generate a key pair for this host. We will use it
	// to obtain a valid host ID.
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%s", address, listenPort)),
		libp2p.Identity(priv),
	}

	basicHost, err := libp2p.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	// Build host multiaddress
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", basicHost.ID().Pretty()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addr := basicHost.Addrs()[0]
	fullAddr := addr.Encapsulate(hostAddr)
	log.Printf("I am %s\n", fullAddr)
	intPort, _ := strconv.ParseInt(listenPort, 10, 32)
	if secio {
		log.Printf("Now run \"go run main.go -l %d -d %s -secio\" on a different terminal\n", intPort+1, fullAddr)
	} else {
		log.Printf("Now run \"go run main.go -l %d -d %s\" on a different terminal\n", intPort+1, fullAddr)
	}

	return basicHost, nil
}

func (p2pManager *P2PManager) Run(target string) {

	if target == "" {
		log.Println("listening for connections")
		// Set a stream handler on host A. /p2p/1.0.0 is
		// a user-defined protocol name.
		p2pManager.BasicHost.SetStreamHandler("/p2p/1.0.0", p2pManager.handleStream)
		// go p2pManager.writeData()

		select {} // hang forever
		/**** This is where the listener code ends ****/
	} else {
		p2pManager.BasicHost.SetStreamHandler("/p2p/1.0.0", p2pManager.handleStream)

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
		fmt.Println("i m peer ", p2pManager.BasicHost.ID())

		// Decapsulate the /ipfs/<peerID> part from the target
		// /ip4/<a.b.c.d>/ipfs/<peer> becomes /ip4/<a.b.c.d>
		targetPeerAddr, _ := ma.NewMultiaddr(
			fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(peerid)))
		targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

		// We have a peer ID and a targetAddr so we add it to the peerstore
		// so LibP2P knows how to contact it
		
		p2pManager.BasicHost.Peerstore().AddAddr(peerid, targetAddr, pstore.PermanentAddrTTL)
		
		
		log.Println("opening stream")
		// make a new stream from host B to host A
		// it should be handled on host A by the handler we set above because
		// we use the same /p2p/1.0.0 protocol
		s, err := p2pManager.BasicHost.NewStream(context.Background(), peerid, "/p2p/1.0.0")
		
		if err != nil {
			log.Fatalln(err)
		}
		
		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
		p2pManager.StreamBuffers = append(p2pManager.StreamBuffers, StreamBuffer{Stream: s, RW: *rw})

		go p2pManager.readData(rw, s)
		go p2pManager.writeData(rw, s)

		select {} // hang forever

	}
}

func (p2pManager *P2PManager) handleStream(s net.Stream) {

	
	//p2pManager.registerStream(s)

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	p2pManager.StreamBuffers = append(p2pManager.StreamBuffers, StreamBuffer{Stream: s, RW: *rw})

	go p2pManager.readData(rw, s)
	go p2pManager.writeData(rw, s)

	log.Println("Got a new stream!")
}

func (p2pManager *P2PManager) appendMsgToBuffer(msgID string) {
	
	if len(p2pManager.HandledMsgBuffer) > p2pManager.HandleMsgLimit {
		p2pManager.HandledMsgBuffer = append(p2pManager.HandledMsgBuffer[1:], msgID)
	} else {
		p2pManager.HandledMsgBuffer = append(p2pManager.HandledMsgBuffer, msgID)
	}

}
func (p2pManager *P2PManager) checkMsgInBuffer(msgID string) bool {

	for _, element := range p2pManager.HandledMsgBuffer {
		if element == msgID { return true }
	}
	return false
}

func (p2pManager *P2PManager) sendMsgToAll(str string) {
	for _, element := range p2pManager.StreamBuffers {
		rw := element.RW
		rw.WriteString(fmt.Sprintf("%s\n", str))
		rw.Flush()
	}
}

func (p2pManager *P2PManager) propagateMsg(str string, s net.Stream) {

	for _, element := range p2pManager.StreamBuffers {
		peerID := element.Stream.Conn().RemotePeer().String()

		if peerID != s.Conn().RemotePeer().String() {
			rw := element.RW
			rw.WriteString(fmt.Sprintf("%s\n", str))
			rw.Flush()
		}
	}
}
func (p2pManager *P2PManager) readData(rw *bufio.ReadWriter, s net.Stream) {

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

				isAleadyReceived := p2pManager.checkMsgInBuffer(p2pMessage.Id)

				if !isAleadyReceived {
					p2pManager.appendMsgToBuffer(p2pMessage.Id)

					switch p2pMessage.Path {
					case types.MSG:
						fmt.Println(string(p2pMessage.Data))
					case types.PUB_SUPERNODE:
						fmt.Println("super node registered with peer id ", s.Conn().RemotePeer().String())
					default:
						fmt.Println("no method ", p2pMessage.Path)
					}
	
					p2pManager.propagateMsg(receivedStr, s)
				} else {
					log.Println("already received msg..")
				}
			}
		}
	}
}

func (p2pManager *P2PManager) writeData(rw *bufio.ReadWriter, s net.Stream) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from stdin")
			panic(err)
		}

		p2pMsg := &types.P2PMessage{
			Id: "",
			Path: types.MSG,
			Data: []byte(sendData),
			Timestamp: uint64(time.Now().UnixNano()),
			Sender: p2pManager.BasicHost.ID().String(),
		}
		p2pMsg.Id = util.CalculateHash(strconv.FormatUint(p2pMsg.Timestamp, 10), strconv.FormatInt(int64(p2pMsg.Path), 10), string(p2pMsg.Data))
		bytes, _ := proto.Marshal(p2pMsg)
		
		str := string(bytes)
		str = strings.Replace(str, "\n", "|bbaa", -1)

		p2pManager.sendMsgToAll(fmt.Sprintf("%s\n", str))
		
		// _, err = rw.WriteString(fmt.Sprintf("%s\n", str))
		// if err != nil {
		// 	fmt.Println("Error writing to buffer")
		// 	panic(err)
		// }
		// err = rw.Flush()
		// if err != nil {
		// 	fmt.Println("Error flushing buffer")
		// 	panic(err)
		// }
	}
}
