package p2p

import (
	"time"
	"fmt"
)
type NodePulse struct {
	NodeHeader	NodeHeader
	LastTimestamp	int64
}
type NodePool struct {
	NodePulses	[]NodePulse
	TimeoutSec	int64
}
func (nodePool *NodePool) AddNodePulse(nodePulse NodePulse) {
	nodePool.NodePulses = append(nodePool.NodePulses, nodePulse)
}

func (nodePool *NodePool) Update(nodeHeader NodeHeader) {
	
	for index, element := range nodePool.NodePulses {
		if element.NodeHeader.PeerId == nodeHeader.PeerId {
			nodePool.NodePulses[index].NodeHeader = nodeHeader
			nodePool.NodePulses[index].LastTimestamp = time.Now().Unix()
			break
		}
	}
}

func (nodePool *NodePool) printAll() {
	for _, element := range nodePool.NodePulses {
		fmt.Println(element)
	}
}

func (nodePool *NodePool) CheckTimeout() {
	
	for index, element := range nodePool.NodePulses {
		if (time.Now().Unix() - element.LastTimestamp) > nodePool.TimeoutSec {
			fmt.Println("remove node ",nodePool.NodePulses[index].NodeHeader.PeerId)
			nodePool.NodePulses = append(nodePool.NodePulses[:index], nodePool.NodePulses[index+1:]...)
			nodePool.CheckTimeout()
			return
		}
	}
	fmt.Println("------------------------Check timeout completed..")
	nodePool.printAll()
}