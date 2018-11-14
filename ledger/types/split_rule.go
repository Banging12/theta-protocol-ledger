package types

import (
	"fmt"

	"github.com/thetatoken/ukulele/common"
)

// ** Split Rule: Specifies the payment split agreement among participating addresses **
//

// Split contains the particiated address and percentage of the payment the address should get
type Split struct {
	Address    common.Address `json:"address"`    // Address to participate in the payment split
	Percentage uint           `json:"percentage"` // An integer between 0 and 100, representing the percentage of the payment the address should get
}

// SplitRule specifies the payment split agreement among differet addresses
type SplitRule struct {
	InitiatorAddress common.Address `json:"initiator_address"` // Address of the initiator
	ResourceID       string         `json:"resource_id"`       // ResourceID of the payment to be split
	Splits           []Split        `json:"splits"`            // Splits of the payments
	EndBlockHeight   uint64         `json:"end_block_height"`  // The block height when the split rule expires
}

func (sc *SplitRule) String() string {
	if sc == nil {
		return "nil-SlashIntent"
	}
	return fmt.Sprintf("SplitRule{%v %v %v %v}",
		sc.InitiatorAddress.Hex(), string(sc.ResourceID), sc.Splits, sc.EndBlockHeight)
}
