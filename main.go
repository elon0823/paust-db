package main

import (
	"flag"

	P2P "github.com/elon0823/paust-db/p2p"
)

func main() {

	host := flag.String("h", "127.0.0.1", "host to connect")
	listenF := flag.String("l", "10000", "wait for incoming connections")
	target := flag.String("d", "", "target peer to dial")
	secio := flag.Bool("secio", false, "enable secio")
	//dbpath := flag.String("dpath", ".pdb", "database path to store")
	seed := flag.Int64("seed", 0, "set random seed for id generation")
	mode := flag.String("m", "", "node mode. (b), (s), (n)")
	//store := flag.Bool("savedb", false, "save db")

	flag.Parse()

	if *mode=="b" {
		bootstrapNode, error := P2P.NewBootstrapNode(*host, *listenF, *secio, *seed)
		if error == nil {
			bootstrapNode.Run()
		}
	} else {
		p2pNode, error := P2P.NewP2PNode(*host, *listenF, *secio, *seed, *mode)
		if error == nil {
			p2pNode.Run(*target)
		}
	}
	

}
