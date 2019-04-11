package p2p

import (
	"github.com/golang/protobuf/proto"
	BC "github.com/elon0823/paust-db/blockchain"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	net "github.com/libp2p/go-libp2p-net"
	mrand "math/rand"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	"crypto/rand"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"log"
	"io"
	"fmt"
	"context"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	"bufio"
	"strconv"
	"sync"
	"os"
	"strings"
	"time"
)

var mutex = &sync.Mutex{}

type StreamBuffer struct {
	Stream net.Stream
	RW bufio.ReadWriter
}
type P2PManager struct {
	Chain *BC.Blockchain
	Address string
	Port string
	BasicHost host.Host
	Secio bool
	Randseed int64
	StreamBuffers []StreamBuffer
}

func NewP2PManager(bchain *BC.Blockchain, address string, listenPort string, secio bool, randseed int64) (*P2PManager, error) {
	host, _ := makeBasicHost(address, listenPort, secio, randseed)
	return &P2PManager{
		Chain: bchain,
		Address: address,
		Port: listenPort,
		BasicHost: host,
		Secio: secio,
		Randseed: randseed,
	}, nil
}

func (p2pManager *P2PManager) StartWebServer(port int) {

	webserver, error := NewWebServer(p2pManager.Chain, p2pManager, "localhost",strconv.Itoa(port),10 * time.Second,1 << 20)
	
	if (error == nil) {
		webserver.Run()
	}
}
func (p2pManager *P2PManager) Run(target string) {

	if target == "" {
		log.Println("listening for connections")
		// Set a stream handler on host A. /p2p/1.0.0 is
		// a user-defined protocol name.
		p2pManager.BasicHost.SetStreamHandler("/p2p/1.0.0", p2pManager.handleStream)
		go p2pManager.writeData()

		p2pManager.StartWebServer(3000)
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

		p2pManager.registerStream(s)
		go p2pManager.writeData()
		p2pManager.requestChain()

		p2pManager.StartWebServer(3000 + mrand.Intn(1000))
		select {} // hang forever

	}
}
func makeBasicHost(address string, listenPort string, secio bool, randseed int64) (host.Host, error) {

	// If the seed is zero, use real cryptographic randomness. Otherwise, use a
	// deterministic randomness source to make generated keys stay the same
	// across multiple runs
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
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%s",address, listenPort)),
		libp2p.Identity(priv),
	}

	// if !secio {
	// 	opts = append(opts, libp2p.NoEncryption())
	// }

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
		log.Printf("Now run \"go run main.go -l %d -d %s -secio\" on a different terminal\n",  intPort+1, fullAddr)
	} else {
		log.Printf("Now run \"go run main.go -l %d -d %s\" on a different terminal\n", intPort+1, fullAddr)
	}

	return basicHost, nil
}

func (p2pManager *P2PManager) registerStream(s net.Stream) {

	
	// Create a buffered stream so that read and writes are non blocking.
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	p2pManager.StreamBuffers = append(p2pManager.StreamBuffers, StreamBuffer{Stream:s, RW:*rw})
	
	// Create a thread to read and write data.
	go p2pManager.readData(rw, s.Conn().RemotePeer().String())
	
}

func (p2pManager *P2PManager) handleStream(s net.Stream) {

	// Create a buffer stream for non blocking read and write.

	p2pManager.registerStream(s)
	
	log.Println("Got a new stream!", "current = ", len(p2pManager.StreamBuffers))
	// for _, element := range p2pManager.StreamBuffers {
	// 	log.Println(element.Stream.Conn().RemotePeer().String())
	// }

	// stream 's' will stay open until you close it (or the other side closes it).
}

func (p2pManager *P2PManager) requestChain() {

	p2pMsg := &P2PMessage{
		Path: "/getChain",
	}

	bytes, _ := proto.Marshal(p2pMsg)

	str := string(bytes)
	str = strings.Replace(str, "\n", "|bbaa", -1)

	for _, element := range p2pManager.StreamBuffers {
		rw := element.RW
		rw.WriteString(fmt.Sprintf("%s\n", str))
		rw.Flush()
		break
	}

}

func (p2pManager *P2PManager) returnChain(fromPeer string) {

	bytes, err := proto.Marshal(p2pManager.Chain)
	if err != nil {
		log.Println(err)
	}

	log.Println("Broadcasting chain for initial sync")

	p2pMsg := &P2PMessage{
		Path: "/broadcastChain",
		Data: bytes,
	}

	bytes, err = proto.Marshal(p2pMsg)

	str := string(bytes)
	str = strings.Replace(str, "\n", "|bbaa", -1)

	for _, element := range p2pManager.StreamBuffers {
		peerID := element.Stream.Conn().RemotePeer().String()
		if peerID == fromPeer {
			rw := element.RW
			rw.WriteString(fmt.Sprintf("%s\n", str))
			rw.Flush()
			return
		}
	}
}

func (p2pManager *P2PManager) BroadcastChain(fromPeer string) {

	bytes, err := proto.Marshal(p2pManager.Chain)
	if err != nil {
		log.Println(err)
	}

	log.Println("Broadcasting chain")

	p2pMsg := &P2PMessage{
		Path: "/broadcastChain",
		Data: bytes,
	}

	bytes, err = proto.Marshal(p2pMsg)

	str := string(bytes)
	str = strings.Replace(str, "\n", "|bbaa", -1)

	for _, element := range p2pManager.StreamBuffers {
		peerID := element.Stream.Conn().RemotePeer().String()
		if peerID != fromPeer {
			rw := element.RW
			rw.WriteString(fmt.Sprintf("%s\n", str))
			rw.Flush()
		}
	}
}
func (p2pManager *P2PManager) readData(rw *bufio.ReadWriter, remotePeer string) {

	for {
		str, err := rw.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		
		if str == "" {
			return
		}
		if( str != "\n") {
			log.Println("from peer ", remotePeer)
			str = strings.Replace(str, "\n", "", -1)
			str = strings.Replace(str, "|bbaa", "\n", -1)
			
			p2pMessage := &P2PMessage {}
			if err := proto.Unmarshal([]byte(str), p2pMessage); err != nil {
				log.Fatal(err)
			} else {
				switch p2pMessage.Path {
				case "/broadcastChain":
					chain := &BC.Blockchain{}
					if err := proto.Unmarshal(p2pMessage.Data, chain); err != nil {
						log.Fatal(err)
					}

					mutex.Lock()
					p2pManager.Chain.ReplaceChain(chain)
					mutex.Unlock()

					p2pManager.BroadcastChain(remotePeer)
				case "/getChain":
					p2pManager.returnChain(remotePeer)

				default:
					fmt.Println("no method ", p2pMessage.Path)
				}	
			}			
		}
	}
}

func (p2pManager *P2PManager) writeData() {

	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		sendData = strings.Replace(sendData, "\n", "", -1)
		bpm, err := strconv.Atoi(sendData)
		if err != nil {
			log.Fatal(err)
		}
		mutex.Lock()
		err = p2pManager.Chain.AddBlock(int32(bpm))
		mutex.Unlock()
		if err != nil {
			log.Println(err)
		}

		p2pManager.BroadcastChain("")

	}
}