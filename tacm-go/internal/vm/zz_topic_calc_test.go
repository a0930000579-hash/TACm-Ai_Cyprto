package vm

import (
	"fmt"
	"testing"

	"tacm/internal/crypto"
)

func TestZzTopicCalc(t *testing.T) {
	sel := func(sig string) []byte { return crypto.Keccak256([]byte(sig))[:4] }
	top := func(sig string) []byte { return crypto.Keccak256([]byte(sig)) }
	fmt.Printf("approve %x\n", sel("approve(address,uint256)"))
	fmt.Printf("setApprovalForAll %x\n", sel("setApprovalForAll(address,bool)"))
	fmt.Printf("getApproved %x\n", sel("getApproved(uint256)"))
	fmt.Printf("isApprovedForAll %x\n", sel("isApprovedForAll(address,address)"))
	fmt.Printf("TopicApproval %x\n", top("Approval(address,address,uint256)"))
	fmt.Printf("TopicApprovalForAll %x\n", top("ApprovalForAll(address,address,bool)"))
}
