package execution

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/thetatoken/ukulele/common"
	"github.com/thetatoken/ukulele/common/result"
	"github.com/thetatoken/ukulele/core"
	"github.com/thetatoken/ukulele/crypto"
	st "github.com/thetatoken/ukulele/ledger/state"
	"github.com/thetatoken/ukulele/ledger/types"
	"github.com/thetatoken/ukulele/store/database/backend"
)

// --------------- Test Utilities --------------- //

type TestConsensusEngine struct {
	privKey *crypto.PrivateKey
}

func (tce *TestConsensusEngine) ID() string                        { return tce.privKey.PublicKey().Address().Hex() }
func (tce *TestConsensusEngine) PrivateKey() *crypto.PrivateKey    { return tce.privKey }
func (tce *TestConsensusEngine) GetTip() *core.ExtendedBlock       { return nil }
func (tce *TestConsensusEngine) GetEpoch() uint64                  { return 100 }
func (tce *TestConsensusEngine) AddMessage(msg interface{})        {}
func (tce *TestConsensusEngine) FinalizedBlocks() chan *core.Block { return nil }

func NewTestConsensusEngine(seed string) *TestConsensusEngine {
	privKey, _, _ := crypto.TEST_GenerateKeyPairWithSeed(seed)
	return &TestConsensusEngine{privKey}
}

type TestValidatorManager struct {
	proposer core.Validator
	valSet   *core.ValidatorSet
}

func (tvm *TestValidatorManager) GetProposerForEpoch(epoch uint64) core.Validator { return tvm.proposer }
func (tvm *TestValidatorManager) GetValidatorSetForEpoch(epoch uint64) *core.ValidatorSet {
	return tvm.valSet
}

func NewTestValidatorManager(proposer core.Validator, valSet *core.ValidatorSet) core.ValidatorManager {
	return &TestValidatorManager{
		proposer: proposer,
		valSet:   valSet,
	}
}

type execTest struct {
	chainID  string
	executor *Executor

	accProposer types.PrivAccount
	accVal2     types.PrivAccount

	accIn  types.PrivAccount
	accOut types.PrivAccount
}

func NewExecTest() *execTest {
	et := &execTest{}
	et.reset()

	return et
}

//reset everything. state is empty
func (et *execTest) reset() {
	et.accIn = types.MakeAccWithInitBalance("foo", types.NewCoins(700000, 50*getMinimumTxFee()))
	et.accOut = types.MakeAccWithInitBalance("bar", types.NewCoins(700000, 50*getMinimumTxFee()))
	et.accProposer = types.MakeAcc("proposer")
	et.accVal2 = types.MakeAcc("val2")

	chainID := "test_chain_id"
	initHeight := uint64(1)
	initRootHash := common.Hash{}
	db := backend.NewMemDatabase()
	ledgerState := st.NewLedgerState(chainID, db)
	ledgerState.ResetState(initHeight, initRootHash)

	consensus := NewTestConsensusEngine("localseed")

	propser := core.NewValidator(et.accProposer.PubKey.ToBytes(), uint64(999))
	val2 := core.NewValidator(et.accVal2.PubKey.ToBytes(), uint64(100))
	valSet := core.NewValidatorSet()
	valSet.AddValidator(propser)
	valSet.AddValidator(val2)
	valMgr := NewTestValidatorManager(propser, valSet)

	executor := NewExecutor(ledgerState, consensus, valMgr)

	et.chainID = chainID
	et.executor = executor
}

func (et *execTest) fastforwardBy(heightIncrement uint64) bool {
	height := et.executor.state.Height()
	rootHash := et.executor.state.Commit()
	et.executor.state.ResetState(height+heightIncrement-1, rootHash)
	return true
}

func (et *execTest) fastforwardTo(targetHeight uint64) bool {
	height := et.executor.state.Height()
	rootHash := et.executor.state.Commit()
	if targetHeight < height+1 {
		return false
	}
	et.executor.state.ResetState(targetHeight, rootHash)
	return true
}

func (et *execTest) signSendTx(tx *types.SendTx, accsIn ...types.PrivAccount) {
	types.SignSendTx(et.chainID, tx, accsIn...)
}

func (et *execTest) state() *st.LedgerState {
	return et.executor.state
}

// returns the final balance and expected balance for input and output accounts
func (et *execTest) execSendTx(tx *types.SendTx, screenTx bool) (res result.Result, inGot, inExp, outGot, outExp types.Coins) {
	initBalIn := et.state().Delivered().GetAccount(et.accIn.Account.PubKey.Address()).Balance
	initBalOut := et.state().Delivered().GetAccount(et.accOut.Account.PubKey.Address()).Balance

	if screenTx {
		_, res = et.executor.ScreenTx(tx)
	} else {
		_, res = et.executor.ExecuteTx(tx)
	}

	endBalIn := et.state().Delivered().GetAccount(et.accIn.Account.PubKey.Address()).Balance
	endBalOut := et.state().Delivered().GetAccount(et.accOut.Account.PubKey.Address()).Balance
	decrBalInExp := tx.Outputs[0].Coins.Plus(tx.Fee) //expected decrease in balance In
	return res, endBalIn, initBalIn.Minus(decrBalInExp), endBalOut, initBalOut.Plus(tx.Outputs[0].Coins)
}

func (et *execTest) acc2State(accs ...types.PrivAccount) {
	for _, acc := range accs {
		et.executor.state.Delivered().SetAccount(acc.Account.PubKey.Address(), &acc.Account)
	}
	et.executor.state.Commit()
}

// Executor returns the executor instance.
func (et *execTest) Executor() *Executor {
	return et.executor
}

// State returns the state instance.
func (et *execTest) State() *st.LedgerState {
	return et.state()
}

// SetAcc saves accounts into state.
func (et *execTest) SetAcc(accs ...types.PrivAccount) {
	et.acc2State(accs...)
}

func getMinimumTxFee() int64 {
	return int64(types.MinimumTransactionFeeGammaWei)
}

func createServicePaymentTx(chainID string, source, target *types.PrivAccount, amount int64, srcSeq, tgtSeq, paymentSeq, reserveSeq int, resourceID string) *types.ServicePaymentTx {
	servicePaymentTx := &types.ServicePaymentTx{
		Fee: types.NewCoins(0, getMinimumTxFee()),
		Source: types.TxInput{
			Address:  source.PubKey.Address(),
			Coins:    types.Coins{GammaWei: big.NewInt(amount), ThetaWei: big.NewInt(0)},
			Sequence: uint64(srcSeq),
		},
		Target: types.TxInput{
			Address:  target.PubKey.Address(),
			Sequence: uint64(tgtSeq),
		},
		PaymentSequence: uint64(paymentSeq),
		ReserveSequence: uint64(reserveSeq),
		ResourceID:      resourceID,
	}

	if srcSeq == 1 {
		servicePaymentTx.Source.PubKey = source.PubKey
	}
	if tgtSeq == 1 {
		servicePaymentTx.Target.PubKey = target.PubKey
	}

	srcSignBytes := servicePaymentTx.SourceSignBytes(chainID)
	servicePaymentTx.Source.Signature = source.Sign(srcSignBytes)

	tgtSignBytes := servicePaymentTx.TargetSignBytes(chainID)
	servicePaymentTx.Target.Signature = target.Sign(tgtSignBytes)

	if !source.PubKey.VerifySignature(srcSignBytes, servicePaymentTx.Source.Signature) {
		panic("Signature verification failed for source")
	}
	if !target.PubKey.VerifySignature(tgtSignBytes, servicePaymentTx.Target.Signature) {
		panic("Signature verification failed for target")
	}

	return servicePaymentTx
}

func setupForServicePayment(ast *assert.Assertions) (et *execTest, resourceID string,
	alice, bob, carol types.PrivAccount, aliceInitBalance, bobInitBalance, carolInitBalance types.Coins) {
	et = NewExecTest()

	alice = types.MakeAcc("User Alice")
	aliceInitBalance = types.Coins{GammaWei: big.NewInt(10000 * getMinimumTxFee()), ThetaWei: big.NewInt(0)}
	alice.Balance = aliceInitBalance
	et.acc2State(alice)
	log.Infof("Alice's pubKey: %v", hex.EncodeToString(alice.PubKey.ToBytes()))
	log.Infof("Alice's Address: %v", alice.PubKey.Address().Hex())

	bob = types.MakeAcc("User Bob")
	bobInitBalance = types.Coins{GammaWei: big.NewInt(3000 * getMinimumTxFee()), ThetaWei: big.NewInt(0)}
	bob.Balance = bobInitBalance
	et.acc2State(bob)
	log.Infof("Bob's pubKey:   %v", hex.EncodeToString(bob.PubKey.ToBytes()))
	log.Infof("Bob's Address: %v", bob.PubKey.Address().Hex())

	carol = types.MakeAcc("User Carol")
	carolInitBalance = types.Coins{GammaWei: big.NewInt(3000 * getMinimumTxFee()), ThetaWei: big.NewInt(0)}
	carol.Balance = carolInitBalance
	et.acc2State(carol)
	log.Infof("Carol's pubKey: %v", hex.EncodeToString(carol.PubKey.ToBytes()))
	log.Infof("Carol's Address: %v", carol.PubKey.Address().Hex())

	et.fastforwardTo(1e2)

	resourceID = "rid001"
	reserveFundTx := &types.ReserveFundTx{
		Fee: types.NewCoins(0, getMinimumTxFee()),
		Source: types.TxInput{
			Address:  alice.PubKey.Address(),
			PubKey:   alice.PubKey,
			Coins:    types.Coins{GammaWei: big.NewInt(1000 * getMinimumTxFee()), ThetaWei: big.NewInt(0)},
			Sequence: 1,
		},
		Collateral:  types.Coins{GammaWei: big.NewInt(1001 * getMinimumTxFee()), ThetaWei: big.NewInt(0)},
		ResourceIDs: []string{resourceID},
		Duration:    1000,
	}
	reserveFundTx.Source.Signature = alice.Sign(reserveFundTx.SignBytes(et.chainID))
	res := et.executor.getTxExecutor(reserveFundTx).sanityCheck(et.chainID, et.state().Delivered(), reserveFundTx)
	ast.True(res.IsOK(), res.String())
	_, res = et.executor.getTxExecutor(reserveFundTx).process(et.chainID, et.state().Delivered(), reserveFundTx)
	ast.True(res.IsOK(), res.String())

	return et, resourceID, alice, bob, carol, aliceInitBalance, bobInitBalance, carolInitBalance
}

type contractByteCode struct {
	DeploymentCode string `json:"deployment_code"`
	Code           string `json:"code"`
}

func loadJSONTest(file string, val interface{}) error {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, val); err != nil {
		if syntaxerr, ok := err.(*json.SyntaxError); ok {
			line := findLine(content, syntaxerr.Offset)
			return fmt.Errorf("JSON syntax error at %v:%v: %v", file, line, err)
		}
		return fmt.Errorf("JSON unmarshal error in %v: %v", file, err)
	}
	return nil
}

func findLine(data []byte, offset int64) (line int) {
	line = 1
	for i, r := range string(data) {
		if int64(i) >= offset {
			return
		}
		if r == '\n' {
			line++
		}
	}
	return
}

func setupForSmartContract(ast *assert.Assertions, numAccounts int) (et *execTest, privAccounts []types.PrivAccount) {
	et = NewExecTest()

	for i := 0; i < numAccounts; i++ {
		secret := "acc_secret_" + strconv.FormatInt(int64(i), 16)
		privAccount := types.MakeAccWithInitBalance(secret, types.NewCoins(0, int64(9000000*types.MinimumGasPrice)))
		privAccounts = append(privAccounts, privAccount)
		et.acc2State(privAccount)
	}
	et.fastforwardTo(1e2)

	return et, privAccounts
}
