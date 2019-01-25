package core

import "github.com/thetatoken/theta/common"

type KVPair struct {
	Key   common.Bytes
	Value common.Bytes
}

type Checkpoint struct {
	FirstBlock  *Block             `rlp:"nil"`
	FirstCC     *CommitCertificate `rlp:"nil"`
	SecondBlock *Block             `rlp:"nil"`
	SecondCC    *CommitCertificate `rlp:"nil"`
	LedgerState []KVPair
}
