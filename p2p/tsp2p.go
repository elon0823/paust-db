package p2p

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"
	"strconv"
	"time"
	"github.com/elon0823/paust-db/types"
	"github.com/elon0823/paust-db/util"
	"github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	net "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

type StreamBuffer struct {
	Stream net.Stream
	RW     bufio.ReadWriter
}

type P2PNode struct {
	BasicHost        host.Host
	Header			 NodeHeader
	StreamBuffers    []StreamBuffer
	HandledMsgBuffer []string
	HandleMsgLimit   int
}

func NewP2PNode(address string, listenPort string, secio bool, randseed int64, mode string) (*P2PNode, error) {

	host, _ := makeBasicHost(address, listenPort, secio, randseed)
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", host.ID().Pretty()))
	addr := host.Addrs()[0]
	fullAddr := addr.Encapsulate(hostAddr)

	header := NodeHeader {
		Host:        	address,
		Port:           listenPort,
		Secio:          secio,
		Randseed:       randseed,
		Mode:			mode,
		PeerId:			host.ID().String(),
		FullAddr:		fullAddr.String(),
	}
	return &P2PNode{
		Header:        header,
		BasicHost:      host,
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

func (p2pNode *P2PNode) Run(target string) {

	if target == "" {
		log.Println("listening for connections")
		// Set a stream handler on host A. /p2p/1.0.0 is
		// a user-defined protocol name.
		p2pNode.BasicHost.SetStreamHandler("/p2p/1.0.0", p2pNode.handleStream)
		// go p2pManager.writeData()

		if p2pNode.Header.Mode == "s" { //super node 
			p2pNode.publishSuperNode()
		} else if p2pNode.Header.Mode == "n" {//normal node
			p2pNode.publishNode()
		}

		select {} // hang forever

	} else {
		p2pNode.BasicHost.SetStreamHandler("/p2p/1.0.0", p2pNode.handleStream)

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
		fmt.Println("i m peer ", p2pNode.BasicHost.ID())

		// Decapsulate the /ipfs/<peerID> part from the target
		// /ip4/<a.b.c.d>/ipfs/<peer> becomes /ip4/<a.b.c.d>
		targetPeerAddr, _ := ma.NewMultiaddr(
			fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(peerid)))
		targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

		// We have a peer ID and a targetAddr so we add it to the peerstore
		// so LibP2P knows how to contact it

		p2pNode.BasicHost.Peerstore().AddAddr(peerid, targetAddr, pstore.PermanentAddrTTL)

		log.Println("opening stream")
		// make a new stream from host B to host A
		// it should be handled on host A by the handler we set above because
		// we use the same /p2p/1.0.0 protocol
		s, err := p2pNode.BasicHost.NewStream(context.Background(), peerid, "/p2p/1.0.0")

		if err != nil {
			log.Fatalln(err)
		}

		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
		p2pNode.StreamBuffers = append(p2pNode.StreamBuffers, StreamBuffer{Stream: s, RW: *rw})

		go p2pNode.readData(rw, s)
		go p2pNode.writeData(rw, s)

		if p2pNode.Header.Mode == "s" { //super node 
			p2pNode.publishSuperNode()
		}
		select {} // hang forever
	}
}

func (p2pNode *P2PNode) removeFromStreamBuffers(s net.Stream) {
	
	for index, element := range p2pNode.StreamBuffers {
		if element.Stream.Conn().RemotePeer().String() == s.Conn().RemotePeer().String() {
			p2pNode.StreamBuffers = append(p2pNode.StreamBuffers[:index], p2pNode.StreamBuffers[index+1:]...)
			break
		}
	}
}
func (p2pNode *P2PNode) handleStream(s net.Stream) {

	//p2pManager.registerStream(s)

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	p2pNode.StreamBuffers = append(p2pNode.StreamBuffers, StreamBuffer{Stream: s, RW: *rw})

	go p2pNode.readData(rw, s)
	go p2pNode.writeData(rw, s)

	log.Println("Got a new stream!")
}

func (p2pNode *P2PNode) appendMsgToBuffer(msgID string) {

	if len(p2pNode.HandledMsgBuffer) > p2pNode.HandleMsgLimit {
		p2pNode.HandledMsgBuffer = append(p2pNode.HandledMsgBuffer[1:], msgID)
	} else {
		p2pNode.HandledMsgBuffer = append(p2pNode.HandledMsgBuffer, msgID)
	}

}
func (p2pNode *P2PNode) checkMsgInBuffer(msgID string) bool {

	for _, element := range p2pNode.HandledMsgBuffer {
		if element == msgID {
			return true
		}
	}
	return false
}

func (p2pNode *P2PNode) sendMsgToAll(str string) {
	for _, element := range p2pNode.StreamBuffers {
		rw := element.RW
		rw.WriteString(fmt.Sprintf("%s\n", str))
		rw.Flush()
	}
}

func (p2pNode *P2PNode) propagateMsg(str string, s net.Stream) {

	for _, element := range p2pNode.StreamBuffers {
		peerID := element.Stream.Conn().RemotePeer().String()

		if peerID != s.Conn().RemotePeer().String() {
			rw := element.RW
			rw.WriteString(fmt.Sprintf("%s\n", str))
			rw.Flush()
		}
	}
}
func (p2pNode *P2PNode) publishNode() {

	s, err := p2pNode.connectToBootstrap()

	if err != nil {
		log.Fatalln(err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	headerBytes, _ := proto.Marshal(&p2pNode.Header)
	bytes := util.MakeMsgBytes(types.PUB_NODE, headerBytes, p2pNode.BasicHost.ID().String()) 

	_, err = rw.WriteString(util.EncapsuleSendMsg(string(bytes)))
	if err != nil {
		fmt.Println("Error writing to buffer")
		panic(err)
	}
	err = rw.Flush()
	if err != nil {
		fmt.Println("Error flushing buffer")
		panic(err)
	}

	log.Println("disconnect from bootstrap")
	rw = nil
	s.Close()

	//heartbeat to bootstrap
	ticker := time.NewTicker(time.Second * 300)
	go func() {
		for range ticker.C {
			p2pNode.startHeartbeatToBootstrap()
		}
	}()
}

func (p2pNode *P2PNode) publishSuperNode() {

	s, err := p2pNode.connectToBootstrap()

	if err != nil {
		log.Fatalln(err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	headerBytes, _ := proto.Marshal(&p2pNode.Header)
	bytes := util.MakeMsgBytes(types.PUB_SUPERNODE, headerBytes, p2pNode.BasicHost.ID().String()) 
	str := util.EncapsuleSendMsg(string(bytes))
	_, err = rw.WriteString(str)
	if err != nil {
		fmt.Println("Error writing to buffer")
		panic(err)
	}
	err = rw.Flush()
	if err != nil {
		fmt.Println("Error flushing buffer")
		panic(err)
	}

	log.Println("disconnect from bootstrap")
	rw = nil
	s.Close()

	p2pNode.sendMsgToAll(str)

	//heartbeat to bootstrap
	ticker := time.NewTicker(time.Second * 10)
	go func() {
		for range ticker.C {
			p2pNode.startHeartbeatToBootstrap()
		}
	}()
}

func (p2pNode *P2PNode) startHeartbeatToBootstrap() {

	s, err := p2pNode.connectToBootstrap()

	if err != nil {
		log.Fatalln(err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
	
	headerBytes, _ := proto.Marshal(&p2pNode.Header) 

	bytes := util.MakeMsgBytes(types.HEARTBEAT, headerBytes, p2pNode.BasicHost.ID().String())

	_, err = rw.WriteString(util.EncapsuleSendMsg(string(bytes)))
	if err != nil {
		fmt.Println("Error writing to buffer")
		panic(err)
	}
	err = rw.Flush()
	if err != nil {
		fmt.Println("Error flushing buffer")
		panic(err)
	}

	log.Println("heartbeat to bootstrap")
	log.Println("disconnect from bootstrap")
	rw = nil
	s.Close()
}

func (p2pNode *P2PNode) connectToBootstrap() (net.Stream, error) {

	selectedBootstrap := BOOTSTRAP_NODES[0]

	p2pNode.BasicHost.SetStreamHandler("/p2p/1.0.0", p2pNode.handleStream)

	ipfsaddr, err := ma.NewMultiaddr(selectedBootstrap)
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

	targetPeerAddr, _ := ma.NewMultiaddr(
		fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(peerid)))
	targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

	p2pNode.BasicHost.Peerstore().AddAddr(peerid, targetAddr, pstore.PermanentAddrTTL)

	log.Println("opening stream to bootstrap")
	return p2pNode.BasicHost.NewStream(context.Background(), peerid, "/p2p/bootstrap/1.0.0")
}

func (p2pNode *P2PNode) readData(rw *bufio.ReadWriter, s net.Stream) {

	for {
		receivedStr, err := rw.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from buffer")
			p2pNode.removeFromStreamBuffers(s)
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

			p2pMessage, err := util.DecapsuleReceiveMsg(receivedStr)

			if err != nil {
				log.Fatal(err)
			} else {
				log.Println("original sender peer ", p2pMessage.Sender)

				isAleadyReceived := p2pNode.checkMsgInBuffer(p2pMessage.Id)

				if !isAleadyReceived {
					p2pNode.appendMsgToBuffer(p2pMessage.Id)

					switch p2pMessage.Path {
					case types.MSG:
						fmt.Println(string(p2pMessage.Data))
					case types.PUB_SUPERNODE:
						nodeHeader := &NodeHeader{}
						if err := proto.Unmarshal([]byte(p2pMessage.Data), nodeHeader); err != nil {
							log.Fatal(err)
						} else {
							fmt.Println("super node registed with peer id = ", nodeHeader.PeerId)
						}
						
					default:
						fmt.Println("no method ", p2pMessage.Path)
					}

					p2pNode.propagateMsg(receivedStr, s)
				} else {
					log.Println("already received msg..")
				}
			}
		}
	}
}

func (p2pNode *P2PNode) writeData(rw *bufio.ReadWriter, s net.Stream) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if sendData == "b\n" {
			fmt.Println("connect to bootstrap")
			p2pNode.connectToBootstrap()
			break
		}
		
		if err != nil {
			fmt.Println("Error reading from stdin")
			p2pNode.removeFromStreamBuffers(s)
			rw = nil
			s.Close()
			break
		}

		bytes := util.MakeMsgBytes(types.MSG, []byte(sendData), p2pNode.BasicHost.ID().String())

		p2pNode.sendMsgToAll(util.EncapsuleSendMsg(string(bytes)))

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
