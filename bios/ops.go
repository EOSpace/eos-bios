package bios

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/eoscanada/eos-bios/bios/unregd"
	eos "github.com/eoscanada/eos-go"
	"github.com/eoscanada/eos-go/ecc"
	"github.com/eoscanada/eos-go/system"
	"github.com/eoscanada/eos-go/token"
)

type Operation interface {
	Actions(b *BIOS) ([]*eos.Action, error)
	ResetTestnetOptions() // TODO: implement the DISABLING of all testnet options when `mainnet` is voted in the `discovery`.
}

var operationsRegistry = map[string]Operation{
	"system.setcode":             &OpSetCode{},
	"system.setram":              &OpSetRAM{},
	"system.newaccount":          &OpNewAccount{},
	"system.setpriv":             &OpSetPriv{},
	"token.create":               &OpCreateToken{},
	"token.issue":                &OpIssueToken{},
	"producers.create_accounts":  &OpCreateProducers{},
	"producers.stake":            &OpStakeProducers{},
	"producers.enrich":           &OpEnrichProducers{},
	"system.setprods":            &OpSetProds{},
	"snapshot.create_accounts":   &OpSnapshotCreateAccounts{},
	"snapshot.load_unregistered": &OpInjectUnregdSnapshot{},
	"system.resign_accounts":     &OpResignAccounts{},
	"system.create_voters":       &OpCreateVoters{},
}

type OperationType struct {
	Op    string
	Label string
	Data  Operation
}

func (o *OperationType) UnmarshalJSON(data []byte) error {
	opData := struct {
		Op    string
		Label string
		Data  json.RawMessage
	}{}
	if err := json.Unmarshal(data, &opData); err != nil {
		return err
	}

	opType, found := operationsRegistry[opData.Op]
	if !found {
		return fmt.Errorf("operation type %q invalid, use one of: %q", opData.Op, operationsRegistry)
	}

	objType := reflect.TypeOf(opType).Elem()
	obj := reflect.New(objType).Interface()

	if len(opData.Data) != 0 {
		err := json.Unmarshal(opData.Data, &obj)
		if err != nil {
			return fmt.Errorf("operation type %q invalid, error decoding: %s", opData.Op, err)
		}
	} //  else {
	// 	_ = json.Unmarshal([]byte("{}"), &obj)
	// }

	opIface, ok := obj.(Operation)
	if !ok {
		return fmt.Errorf("operation type %q isn't an op", opData.Op)
	}

	*o = OperationType{
		Op:    opData.Op,
		Label: opData.Label,
		Data:  opIface,
	}

	return nil
}

//

type OpSetCode struct {
	Account         eos.AccountName
	ContractNameRef string `json:"contract_name_ref"`
	IsMainnet       bool
}

func (op *OpSetCode) ResetTestnetOptions() {
	op.IsMainnet = true
}

func (op *OpSetCode) Actions(b *BIOS) (out []*eos.Action, err error) {
	if op.IsMainnet && op.Account == eos.AccountName("eosio.disco") {
		return
	}

	wasmFileRef, err := b.GetContentsCacheRef(fmt.Sprintf("%s.wasm", op.ContractNameRef))
	if err != nil {
		return nil, err
	}
	abiFileRef, err := b.GetContentsCacheRef(fmt.Sprintf("%s.abi", op.ContractNameRef))
	if err != nil {
		return nil, err
	}

	setCode, err := system.NewSetCodeTx(
		op.Account,
		b.Network.FileNameFromCache(wasmFileRef),
		b.Network.FileNameFromCache(abiFileRef),
	)
	if err != nil {
		return nil, fmt.Errorf("NewSetCodeTx %s: %s", op.ContractNameRef, err)
	}

	return setCode.Actions, nil
}

//

type OpSetRAM struct {
	MaxRAMSize uint64 `json:"max_ram_size"`
}

func (op *OpSetRAM) ResetTestnetOptions() { return }
func (op *OpSetRAM) Actions(b *BIOS) (out []*eos.Action, err error) {
	return append(out, system.NewSetRAM(op.MaxRAMSize)), nil
}

//

type OpNewAccount struct {
	Creator    eos.AccountName
	NewAccount eos.AccountName `json:"new_account"`
	Pubkey     string
	IsMainnet  bool
}

func (op *OpNewAccount) ResetTestnetOptions() {
	op.IsMainnet = true
}

func (op *OpNewAccount) Actions(b *BIOS) (out []*eos.Action, err error) {
	if op.IsMainnet && op.NewAccount == eos.AccountName("eosio.disco") {
		return
	}

	pubKey := b.EphemeralPublicKey

	if op.Pubkey != "ephemeral" {
		pubKey, err = ecc.NewPublicKey(op.Pubkey)
		if err != nil {
			return nil, fmt.Errorf("reading pubkey: %s", err)
		}
	}

	return append(out, system.NewNewAccount(op.Creator, op.NewAccount, pubKey)), nil
}

type OpCreateVoters struct {
	Creator eos.AccountName
	Pubkey  string
	Count   int
}

func (op *OpCreateVoters) ResetTestnetOptions() { return }
func (op *OpCreateVoters) Actions(b *BIOS) (out []*eos.Action, err error) {
	pubKey := b.EphemeralPublicKey

	if op.Pubkey != "ephemeral" {
		pubKey, err = ecc.NewPublicKey(op.Pubkey)
		if err != nil {
			return nil, fmt.Errorf("reading pubkey: %s", err)
		}
	}

	for i := 0; i < op.Count; i++ {
		voterName := eos.AccountName(voterName(i))
		fmt.Println("Creating voter: ", voterName)
		out = append(out, system.NewNewAccount(op.Creator, voterName, pubKey))
		out = append(out, token.NewTransfer(op.Creator, voterName, eos.NewEOSAsset(1000000000), ""))
		out = append(out, system.NewBuyRAMBytes(AN("eosio"), voterName, 8192)) // 8kb gift !
		out = append(out, system.NewDelegateBW(AN("eosio"), voterName, eos.NewEOSAsset(10000), eos.NewEOSAsset(10000), true))

	}

	return
}

const charset = "abcdefghijklmnopqrstuvwxyz"

func voterName(index int) string {
	padding := string(bytes.Repeat([]byte{charset[index]}, 7))
	return "voter" + padding
}

type OpSetPriv struct {
	Account eos.AccountName
}

func (op *OpSetPriv) ResetTestnetOptions() { return }
func (op *OpSetPriv) Actions(b *BIOS) (out []*eos.Action, err error) {
	return append(out, system.NewSetPriv(op.Account)), nil
}

//

type OpCreateToken struct {
	Account eos.AccountName `json:"account"`
	Amount  eos.Asset       `json:"amount"`
}

func (op *OpCreateToken) ResetTestnetOptions() {}
func (op *OpCreateToken) Actions(b *BIOS) (out []*eos.Action, err error) {
	act := token.NewCreate(op.Account, op.Amount)
	return append(out, act), nil
}

//

type OpIssueToken struct {
	Account eos.AccountName
	Amount  eos.Asset
	Memo    string
}

func (op *OpIssueToken) ResetTestnetOptions() {}
func (op *OpIssueToken) Actions(b *BIOS) (out []*eos.Action, err error) {
	act := token.NewIssue(op.Account, op.Amount, op.Memo)
	return append(out, act), nil
}

//

type OpCreateProducers struct {
	IsMainnet bool
}

func (op *OpCreateProducers) ResetTestnetOptions() {
	op.IsMainnet = true
}

func (op *OpCreateProducers) Actions(b *BIOS) (out []*eos.Action, err error) {
	if op.IsMainnet {
		return
	}

	for _, prod := range b.ShuffledProducers {
		prodName := prod.Discovery.TargetAccountName
		if prodName == AN("eosio") {
			prodName = prod.Discovery.SeedNetworkAccountName // only happens with --single
		}

		newAccount := system.NewNewAccount(AN("eosio"), prodName, ecc.PublicKey{}) // overridden just below
		newAccount.ActionData = eos.NewActionData(system.NewAccount{
			Creator: AN("eosio"),
			Name:    prodName,
			Owner:   prod.Discovery.TargetInitialAuthority.Owner,
			Active:  prod.Discovery.TargetInitialAuthority.Active,
		})
		out = append(out, newAccount, nil)
	}
	return
}

//

type OpStakeProducers struct {
	IsMainnet bool
}

func (op *OpStakeProducers) ResetTestnetOptions() {
	op.IsMainnet = true
}

func (op *OpStakeProducers) Actions(b *BIOS) (out []*eos.Action, err error) {
	if op.IsMainnet {
		return
	}

	for _, prod := range b.ShuffledProducers {
		prodName := prod.Discovery.TargetAccountName
		if prodName == AN("eosio") {
			prodName = prod.Discovery.SeedNetworkAccountName // only happens with --single
		}

		buyRAMBytes := system.NewBuyRAMBytes(AN("eosio"), prodName, 8192) // 8kb gift !
		delegateBW := system.NewDelegateBW(AN("eosio"), prodName, eos.NewEOSAsset(100000), eos.NewEOSAsset(100000), true)

		out = append(out, buyRAMBytes, delegateBW, nil)
	}
	return
}

//

type OpEnrichProducers struct {
	// TestnetEnrichProducers will provide each producer account with some EOS, only on testnets.
	TestnetEnrichProducers bool `json:"TESTNET_ENRICH_PRODUCERS"`
}

func (op *OpEnrichProducers) ResetTestnetOptions() {
	op.TestnetEnrichProducers = false
}

func (op *OpEnrichProducers) Actions(b *BIOS) (out []*eos.Action, err error) {
	if !op.TestnetEnrichProducers {
		return
	}

	for _, prod := range b.ShuffledProducers {
		prodName := prod.Discovery.TargetAccountName
		if prodName == AN("eosio") {
			prodName = prod.Discovery.SeedNetworkAccountName // only happens with --single
		}

		b.Log.Debugf("- DEBUG: Enriching producer %q\n", prodName)

		act := token.NewIssue(prodName, eos.NewEOSAsset(100000000000), "To play around") // You need to be 15 to unlock the chain with that amount.
		out = append(out, act, nil)
	}
	return
}

//

type OpSnapshotCreateAccounts struct {
	BuyRAM                  uint64 `json:"buy_ram_bytes"`
	TestnetTruncateSnapshot int    `json:"TESTNET_TRUNCATE_SNAPSHOT"`
}

func (op *OpSnapshotCreateAccounts) ResetTestnetOptions() {
	op.TestnetTruncateSnapshot = 0
}

func (op *OpSnapshotCreateAccounts) Actions(b *BIOS) (out []*eos.Action, err error) {
	snapshotFile, err := b.GetContentsCacheRef("snapshot.csv")
	if err != nil {
		return nil, err
	}

	rawSnapshot, err := b.Network.ReadFromCache(snapshotFile)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot file: %s", err)
	}

	snapshotData, err := NewSnapshot(rawSnapshot)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot csv: %s", err)
	}

	if len(snapshotData) == 0 {
		return nil, fmt.Errorf("snapshot is empty or not loaded")
	}

	wellKnownPubkey, _ := ecc.NewPublicKey("EOS6MRyAjQq8ud7hVNYcfnVPJqcVpscN5So8BhtHuGYqET5GDW5CV")

	for idx, hodler := range snapshotData {
		if trunc := op.TestnetTruncateSnapshot; trunc != 0 {
			if idx == trunc {
				b.Log.Debugf("- DEBUG: truncated snapshot to %d rows\n", trunc)
				break
			}
		}

		destAccount := AN(hodler.AccountName)
		destPubKey := hodler.EOSPublicKey
		if b.HackVotingAccounts {
			destPubKey = wellKnownPubkey
		}

		// we should have created the account before loading `eosio.system`, otherwise
		// b1 wouldn't have been accepted.
		if hodler.EthereumAddress != "0x00000000000000000000000000000000000000b1" {
			// create all other accounts, but not `b1`.. because it's a short name..
			out = append(out, system.NewNewAccount(AN("eosio"), destAccount, destPubKey))
		}

		cpuStake, netStake, rest := splitSnapshotStakes(hodler.Balance)

		// special case `transfer` for `b1` ?
		out = append(out, system.NewDelegateBW(AN("eosio"), destAccount, cpuStake, netStake, true))
		out = append(out, system.NewBuyRAMBytes(AN("eosio"), destAccount, uint32(op.BuyRAM)))
		out = append(out, nil) // end transaction

		memo := "Welcome " + hodler.EthereumAddress[len(hodler.EthereumAddress)-6:]
		out = append(out, token.NewTransfer(AN("eosio"), destAccount, rest, memo), nil)
	}

	return
}

func splitSnapshotStakes(balance eos.Asset) (cpu, net, xfer eos.Asset) {
	if balance.Amount < 5000 {
		return
	}

	// everyone has minimum 0.25 EOS staked
	// some 10 EOS unstaked
	// the rest split between the two

	cpu = eos.NewEOSAsset(2500)
	net = eos.NewEOSAsset(2500)

	remainder := eos.NewEOSAsset(balance.Amount - cpu.Amount - net.Amount)

	if remainder.Amount <= 100000 /* 10.0 EOS */ {
		return cpu, net, remainder
	}

	remainder.Amount -= 100000 // keep them floating, unstaked

	firstHalf := remainder.Amount / 2
	cpu.Amount += firstHalf
	net.Amount += remainder.Amount - firstHalf

	return cpu, net, eos.NewEOSAsset(100000)
}

//

type OpInjectUnregdSnapshot struct {
	TestnetTruncateSnapshot int `json:"TESTNET_TRUNCATE_SNAPSHOT"`
}

func (op *OpInjectUnregdSnapshot) ResetTestnetOptions() {
	op.TestnetTruncateSnapshot = 0
}

func (op *OpInjectUnregdSnapshot) Actions(b *BIOS) (out []*eos.Action, err error) {
	snapshotFile, err := b.GetContentsCacheRef("snapshot_unregistered.csv")
	if err != nil {
		return nil, err
	}

	rawSnapshot, err := b.Network.ReadFromCache(snapshotFile)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot file: %s", err)
	}

	snapshotData, err := NewUnregdSnapshot(rawSnapshot)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot csv: %s", err)
	}

	if len(snapshotData) == 0 {
		return nil, fmt.Errorf("snapshot is empty or not loaded")
	}

	for idx, hodler := range snapshotData {
		if trunc := op.TestnetTruncateSnapshot; trunc != 0 {
			if idx == trunc {
				b.Log.Debugf("- DEBUG: truncated unreg'd snapshot to %d rows\n", trunc)
				break
			}
		}

		//system.NewDelegatedNewAccount(AN("eosio"), AN(hodler.AccountName), AN("eosio.unregd"))

		out = append(out,
			unregd.NewAdd(hodler.EthereumAddress, hodler.Balance),
			token.NewTransfer(AN("eosio"), AN("eosio.unregd"), hodler.Balance, "Future claim"),
			nil,
		)
	}

	return
}

//

type OpSetProds struct{}

func (op *OpSetProds) ResetTestnetOptions() {}

func (op *OpSetProds) Actions(b *BIOS) (out []*eos.Action, err error) {
	// We he can at least process the last few blocks, that wrap up
	// and resigns the system accounts.
	prodkeys := []system.ProducerKey{system.ProducerKey{
		ProducerName:    AN("eosio"),
		BlockSigningKey: b.EphemeralPublicKey,
	}}

	// WARN: this makes it a SOLO producer on mainnet.

	//prodkeys := []system.ProducerKey{}
	for idx, prod := range b.ShuffledProducers {
		if idx == 0 {
			continue
		}
		targetKey := prod.Discovery.TargetAppointedBlockProducerSigningKey
		targetAcct := prod.Discovery.TargetAccountName
		if targetAcct == AN("eosio") {
			targetKey = b.EphemeralPublicKey
		}
		prodkeys = append(prodkeys, system.ProducerKey{targetAcct, targetKey})
		if len(prodkeys) >= 21 {
			break
		}
	}

	out = append(out, system.NewSetProds(prodkeys))

	return
}

//

type OpResignAccounts struct {
	Accounts            []eos.AccountName
	TestnetKeepAccounts bool `json:"TESTNET_KEEP_ACCOUNTS"`
	IsMainnet           bool
}

func (op *OpResignAccounts) ResetTestnetOptions() {
	op.TestnetKeepAccounts = false
	op.IsMainnet = true
}

func (op *OpResignAccounts) Actions(b *BIOS) (out []*eos.Action, err error) {
	if op.TestnetKeepAccounts {
		b.Log.Debugln("DEBUG: Keeping system accounts around, for testing purposes.")
		return
	}

	systemAccount := AN("eosio")
	prodsAccount := AN("eosio.prods") // this is a special system account that is granted by 2/3 + 1 of the current BP schedule.

	//nullKey := ecc.PublicKey{Curve: ecc.CurveK1, Content: make([]byte, 33, 33)}
	for _, acct := range op.Accounts {
		if acct == systemAccount || (op.IsMainnet && acct == eos.AccountName("eosio.disco")) {
			continue // special treatment for `eosio` below
		}
		out = append(out,
			system.NewUpdateAuth(acct, PN("active"), PN("owner"), eos.Authority{
				Threshold: 1,
				Accounts: []eos.PermissionLevelWeight{
					eos.PermissionLevelWeight{
						Permission: eos.PermissionLevel{
							Actor:      AN("eosio"),
							Permission: PN("active"),
						},
						Weight: 1,
					},
				},
			}, PN("active")),
			system.NewUpdateAuth(acct, PN("owner"), PN(""), eos.Authority{
				Threshold: 1,
				Accounts: []eos.PermissionLevelWeight{
					eos.PermissionLevelWeight{
						Permission: eos.PermissionLevel{
							Actor:      AN("eosio"),
							Permission: PN("active"),
						},
						Weight: 1,
					},
				},
			}, PN("owner")),
		)
	}

	out = append(out,
		system.NewUpdateAuth(systemAccount, PN("active"), PN("owner"), eos.Authority{
			Threshold: 1,
			Accounts: []eos.PermissionLevelWeight{
				eos.PermissionLevelWeight{
					Permission: eos.PermissionLevel{
						Actor:      prodsAccount,
						Permission: PN("active"),
					},
					Weight: 1,
				},
			},
		}, PN("active")),
		system.NewUpdateAuth(systemAccount, PN("owner"), PN(""), eos.Authority{
			Threshold: 1,
			Accounts: []eos.PermissionLevelWeight{
				eos.PermissionLevelWeight{
					Permission: eos.PermissionLevel{
						Actor:      prodsAccount,
						Permission: PN("active"),
					},
					Weight: 1,
				},
			},
		}, PN("owner")),
	)

	out = append(out, nil)

	return
}
