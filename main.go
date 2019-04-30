package main

import (
	P2P "github.com/elon0823/paust-db/p2p"
)

func main() {

	p2pManager, error := P2P.NewP2PManager()
	if error == nil {
		p2pManager.Run()
	}

}