package state

import (
	"bytes"
	"fmt"
	"math/big"

	log "github.com/sirupsen/logrus"
	"github.com/thetatoken/ukulele/common"
	"github.com/thetatoken/ukulele/core"
	"github.com/thetatoken/ukulele/crypto"
	"github.com/thetatoken/ukulele/ledger/types"
	"github.com/thetatoken/ukulele/rlp"
	"github.com/thetatoken/ukulele/store/database"
	"github.com/thetatoken/ukulele/store/treestore"
)

//
// ------------------------- StoreView -------------------------
//

type StoreView struct {
	height uint64 // block height
	store  *treestore.TreeStore

	coinbaseTransactinProcessed bool
	slashIntents                []types.SlashIntent
	validatorsDiff              []*core.Validator
	refund                      uint64 // Gas refund during smart contract execution
}

// NewStoreView creates an instance of the StoreView
func NewStoreView(height uint64, root common.Hash, db database.Database) *StoreView {
	store := treestore.NewTreeStore(root, db)
	if store == nil {
		return nil
	}
	sv := &StoreView{
		height:         height,
		store:          store,
		slashIntents:   []types.SlashIntent{},
		validatorsDiff: []*core.Validator{},
		refund:         0,
	}
	return sv
}

// Copy returns a copy of the StoreView
func (sv *StoreView) Copy() (*StoreView, error) {
	copiedStore, err := sv.store.Copy()
	if err != nil {
		return nil, err
	}
	copiedStoreView := &StoreView{
		height:         sv.height,
		store:          copiedStore,
		slashIntents:   []types.SlashIntent{},
		validatorsDiff: []*core.Validator{},
		refund:         0,
	}
	return copiedStoreView, nil
}

// Hash returns the root hash of the tree store
func (sv *StoreView) Hash() common.Hash {
	return sv.store.Hash()
}

// Height returns the block height corresponding to the stored state
func (sv *StoreView) Height() uint64 {
	return sv.height
}

// IncrementHeight increments the block height by 1
func (sv *StoreView) IncrementHeight() {
	sv.height++
}

// Save saves the StoreView to the persistent storage, and return the root hash
func (sv *StoreView) Save() common.Hash {
	rootHash, err := sv.store.Commit()
	if err != nil {
		panic(fmt.Sprintf("Failed to save the StoreView: %v", err))
	}
	sv.store.Trie.GetDB().Commit(rootHash, true)
	return rootHash
}

// Get returns the value corresponding the key
func (sv *StoreView) Get(key common.Bytes) common.Bytes {
	value := sv.store.Get(key)
	return value
}

// Delete removes the value corresponding the key
func (sv *StoreView) Delete(key common.Bytes) {
	sv.store.Delete(key)
}

// Set returns the value corresponding the key
func (sv *StoreView) Set(key common.Bytes, value common.Bytes) {
	sv.store.Set(key, value)
}

// AddSlashIntent adds slashIntent
func (sv *StoreView) AddSlashIntent(slashIntent types.SlashIntent) {
	sv.slashIntents = append(sv.slashIntents, slashIntent)
}

// GetSlashIntents retrieves all the slashIntents
func (sv *StoreView) GetSlashIntents() []types.SlashIntent {
	return sv.slashIntents
}

// ClearSlashIntents clears all the slashIntents
func (sv *StoreView) ClearSlashIntents() {
	sv.slashIntents = []types.SlashIntent{}
}

// CoinbaseTransactinProcessed returns whether the coinbase transaction for the current block has been processed
func (sv *StoreView) CoinbaseTransactinProcessed() bool {
	return sv.coinbaseTransactinProcessed
}

// SetCoinbaseTransactionProcessed sets whether the coinbase transaction for the current block has been processed
func (sv *StoreView) SetCoinbaseTransactionProcessed(processed bool) {
	sv.coinbaseTransactinProcessed = processed
}

// GetAndClearValidatorDiff retrives and clear validator diff
func (sv *StoreView) GetAndClearValidatorDiff() []*core.Validator {
	res := sv.validatorsDiff
	sv.validatorsDiff = nil
	return res
}

// SetValidatorDiff set validator diff
func (sv *StoreView) SetValidatorDiff(diff []*core.Validator) {
	sv.validatorsDiff = diff
}

// GetAccount returns an account.
func (sv *StoreView) GetAccount(addr common.Address) *types.Account {
	data := sv.Get(AccountKey(addr))
	if data == nil || len(data) == 0 {
		return nil
	}
	acc := &types.Account{}
	err := types.FromBytes(data, acc)
	if err != nil {
		panic(fmt.Sprintf("Error reading account %X error: %v",
			data, err.Error()))
	}
	return acc
}

// SetAccount sets an account.
func (sv *StoreView) SetAccount(addr common.Address, acc *types.Account) {
	accBytes, err := types.ToBytes(acc)
	if err != nil {
		panic(fmt.Sprintf("Error writing account %v error: %v",
			acc, err.Error()))
	}
	sv.Set(AccountKey(addr), accBytes)
}

// DeleteAccount deletes an account.
func (sv *StoreView) DeleteAccount(addr common.Address) {
	sv.Delete(AccountKey(addr))
}

// SplitRuleExists checks if a split rule associated with the given resourceID already exists
func (sv *StoreView) SplitRuleExists(resourceID string) bool {
	return sv.GetSplitRule(resourceID) != nil
}

// AddSplitRule adds a split rule
func (sv *StoreView) AddSplitRule(splitRule *types.SplitRule) bool {
	if sv.SplitRuleExists(splitRule.ResourceID) {
		return false // Each resourceID can have at most one corresponding split rule
	}

	sv.SetSplitRule(splitRule.ResourceID, splitRule)
	return true
}

// UpdateSplitRule updates a split rule
func (sv *StoreView) UpdateSplitRule(splitRule *types.SplitRule) bool {
	if !sv.SplitRuleExists(splitRule.ResourceID) {
		return false
	}

	sv.SetSplitRule(splitRule.ResourceID, splitRule)
	return true
}

// GetSplitRule gets split rule.
func (sv *StoreView) GetSplitRule(resourceID string) *types.SplitRule {
	data := sv.Get(SplitRuleKey(resourceID))
	if data == nil || len(data) == 0 {
		return nil
	}
	splitRule := &types.SplitRule{}
	err := types.FromBytes(data, splitRule)
	if err != nil {
		panic(fmt.Sprintf("Error reading splitRule %X error: %v",
			data, err.Error()))
	}
	return splitRule
}

// SetSplitRule sets split rule.
func (sv *StoreView) SetSplitRule(resourceID string, splitRule *types.SplitRule) {
	splitRuleBytes, err := types.ToBytes(splitRule)
	if err != nil {
		panic(fmt.Sprintf("Error writing splitRule %v error: %v",
			splitRule, err.Error()))
	}
	sv.Set(SplitRuleKey(resourceID), splitRuleBytes)
}

// DeleteSplitRule deletes a split rule.
func (sv *StoreView) DeleteSplitRule(resourceID string) bool {
	key := SplitRuleKey(resourceID)
	deleted := sv.store.Delete(key)
	return deleted
}

// DeleteExpiredSplitRules deletes a split rule.
func (sv *StoreView) DeleteExpiredSplitRules(currentBlockHeight uint64) bool {
	prefix := SplitRuleKeyPrefix()

	expiredKeys := []common.Bytes{}
	sv.store.Traverse(prefix, func(key, value common.Bytes) bool {
		var splitRule types.SplitRule
		err := types.FromBytes(value, &splitRule)
		if err != nil {
			panic(fmt.Sprintf("Error reading splitRule %X error: %v", value, err.Error()))
		}

		expired := (splitRule.EndBlockHeight < currentBlockHeight)
		if expired {
			expiredKeys = append(expiredKeys, key)
		}
		return true
	})

	for _, key := range expiredKeys {
		deleted := sv.store.Delete(key)
		if !deleted {
			log.Errorf("Failed to delete expired split rules")
			return false
		}
	}

	return true
}

func (sv *StoreView) GetStore() *treestore.TreeStore {
	return sv.store
}

//
// ---------- Implement vm.StateDB interface -----------
//

func (sv *StoreView) CreateAccount(addr common.Address) {
	account := types.NewAccount()
	sv.SetAccount(addr, account)
}

func (sv *StoreView) GetOrCreateAccount(addr common.Address) *types.Account {
	account := sv.GetAccount(addr)
	if account != nil {
		return account
	}
	return types.NewAccount()
}

func (sv *StoreView) SubBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	account := sv.GetAccount(addr)
	account.Balance = account.Balance.NoNil()
	account.Balance.GammaWei.Sub(account.Balance.GammaWei, amount)
	sv.SetAccount(addr, account)
}

func (sv *StoreView) AddBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	account := sv.GetAccount(addr)
	account.Balance = account.Balance.NoNil()
	account.Balance.GammaWei.Add(account.Balance.GammaWei, amount)
	sv.SetAccount(addr, account)
}

func (sv *StoreView) GetBalance(addr common.Address) *big.Int {
	return sv.GetOrCreateAccount(addr).Balance.GammaWei
}

func (sv *StoreView) GetNonce(addr common.Address) uint64 {
	return sv.GetOrCreateAccount(addr).Sequence
}

func (sv *StoreView) SetNonce(addr common.Address, nonce uint64) {
	account := sv.GetOrCreateAccount(addr)
	account.Sequence = nonce
	sv.SetAccount(addr, account)
}

func (sv *StoreView) GetCodeHash(addr common.Address) common.Hash {
	account := sv.GetAccount(addr)
	if account == nil {
		return common.Hash{}
	}
	return account.CodeHash
}

func (sv *StoreView) GetCode(addr common.Address) []byte {
	account := sv.GetAccount(addr)
	if account == nil {
		return nil
	}
	if account.CodeHash == types.EmptyCodeHash {
		return nil
	}
	return sv.GetCodeByHash(account.CodeHash)
}

func (sv *StoreView) GetCodeByHash(codeHash common.Hash) []byte {
	codeKey := CodeKey(codeHash[:])
	return sv.Get(codeKey)
}

func (sv *StoreView) SetCode(addr common.Address, code []byte) {
	account := sv.GetOrCreateAccount(addr)
	codeHash := crypto.Keccak256Hash(code)
	account.CodeHash = codeHash
	sv.Set(CodeKey(account.CodeHash[:]), code)
	sv.SetAccount(addr, account)
}

func (sv *StoreView) GetCodeSize(addr common.Address) int {
	return len(sv.GetCode(addr))

}

func (sv *StoreView) AddRefund(gas uint64) {
	sv.refund += gas
}

func (sv *StoreView) SubRefund(gas uint64) {
	if gas > sv.refund {
		panic("Refund counter below zero")
	}
	sv.refund -= gas
}

func (sv *StoreView) GetRefund() uint64 {
	return sv.refund
}

func (sv *StoreView) ResetRefund() {
	sv.refund = 0
}

func (sv *StoreView) GetCommittedState(addr common.Address, key common.Hash) common.Hash {
	return sv.GetState(addr, key)
}

func (sv *StoreView) getAccountStorage(account *types.Account) *treestore.TreeStore {
	return treestore.NewTreeStore(account.Root, sv.store.GetDB())
}

func (sv *StoreView) GetState(addr common.Address, key common.Hash) common.Hash {
	account := sv.GetAccount(addr)
	if account == nil {
		return common.Hash{}
	}
	enc, err := sv.getAccountStorage(account).TryGet(key[:])
	if err != nil {
		panic(err)
	}
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			panic(err)
		}
		return common.BytesToHash(content)
	}
	return common.Hash{}
}

func (sv *StoreView) SetState(addr common.Address, key, val common.Hash) {
	account := sv.GetAccount(addr)
	if account == nil {
		account = types.NewAccount()
	}
	tree := sv.getAccountStorage(account)
	if (val == common.Hash{}) {
		tree.TryDelete(key[:])
		return
	}
	// Encoding []byte cannot fail, ok to ignore the error.
	v, _ := rlp.EncodeToBytes(bytes.TrimLeft(val[:], "\x00"))
	tree.TryUpdate(key[:], v)
	root, err := tree.Commit()
	if err != nil {
		panic(err)
	}

	account.Root = root
	sv.SetAccount(addr, account)
}

func (sv *StoreView) Suicide(addr common.Address) bool {
	if sv.GetAccount(addr) == nil {
		return false
	}
	sv.DeleteAccount(addr)
	return true
}

func (sv *StoreView) HasSuicided(addr common.Address) bool {
	account := sv.GetAccount(addr)
	return account == nil
}

// Exist reports whether the given account exists in state.
// Notably this should also return true for suicided accounts.
func (sv *StoreView) Exist(addr common.Address) bool {
	account := sv.GetAccount(addr)
	return account != nil
}

// Empty returns whether the given account is empty. Empty
// is defined according to EIP161 (balance = nonce = code = 0).
func (sv *StoreView) Empty(addr common.Address) bool {
	account := sv.GetAccount(addr)
	if account == nil {
		return true
	}
	return account.Sequence == 0 &&
		account.CodeHash == types.EmptyCodeHash &&
		account.Balance.IsZero()
}

func (sv *StoreView) RevertToSnapshot(root common.Hash) {
	var err error
	sv.store, err = sv.store.Revert(root) // revert to one of the previous roots
	if err != nil {
		panic(err)
	}
}

func (sv *StoreView) Snapshot() common.Hash {
	sv.store.Trie.Commit(nil) // Needs to commit to the in-memory trie DB
	return sv.store.Hash()
}

func (sv *StoreView) AddLog(*types.Log) {
	// TODO
}
