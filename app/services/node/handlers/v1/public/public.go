// Package public maintains the group of handlers for public access.
package public

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/ardanlabs/blockchain/business/web/v1"
	"github.com/ardanlabs/blockchain/foundation/blockchain/database"
	"github.com/ardanlabs/blockchain/foundation/blockchain/state"
	"github.com/ardanlabs/blockchain/foundation/nameservice"
	"github.com/ardanlabs/blockchain/foundation/web"
	"go.uber.org/zap"
)

// Handlers manages the set of bar ledger endpoints.
type Handlers struct {
	Log   *zap.SugaredLogger
	State *state.State
	NS    *nameservice.NameService
	// WS    websocket.Upgrader
	// Evts *events.Events
}

func (h Handlers) StartMining(ctx context.Context, w http.ResponseWriter, r *http.Request) error {

	block, err := h.State.MineNewBlock(ctx)
	if err != nil {
		return v1.NewRequestError(err, http.StatusBadRequest)
	}

	h.Log.Infow("================================")
	h.Log.Infow("Mined Block", "BLOCK", block.Header)
	h.Log.Infow("Mined Block", "BLOCK HASH", block.Hash())
	h.Log.Infow("Mined Block", "BLOCK Nonce", block.Header.Nonce)
	h.Log.Infow("================================")

	resp := struct {
		Status string
	}{
		Status: "OK",
	}

	return web.Respond(ctx, w, resp, http.StatusOK)
}

// SubmitWalletTransaction adds new transactions to the mempool.
func (h Handlers) SubmitWalletTransaction(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	v, err := web.GetValues(ctx)
	if err != nil {
		return web.NewShutdownError("web value missing from context")
	}

	// Decode the JSON in the post call into a Signed transaction.
	var signedTx database.SignedTx
	if err := web.Decode(r, &signedTx); err != nil {
		return fmt.Errorf("unable to decode payload: %w", err)
	}

	h.Log.Infow("add tran", "traceid", v.TraceID, "sig:nonce", signedTx, "from", signedTx.FromID, "to", signedTx.ToID, "value", signedTx.Value, "tip", signedTx.Tip)

	// Ask the state package to add this transaction to the mempool. Only the
	// checks are the transaction signature and the recipient account format.
	// It's up to the wallet to make sure the account has a proper balance and
	// nonce. Fees will be taken if this transaction is mined into a block.
	if err := h.State.UpsertWalletTransaction(signedTx); err != nil {
		return v1.NewRequestError(err, http.StatusBadRequest)
	}

	resp := struct {
		Status string `json:"status"`
	}{
		Status: "transactions added to mempool",
	}

	return web.Respond(ctx, w, resp, http.StatusOK)
}

// Mempool returns the set of uncommitted transactions.
func (h Handlers) Mempool(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	acct := web.Param(r, "account")

	mempool := h.State.RetriveMempool()

	trans := []tx{}
	for _, tran := range mempool {
		if acct != "" && ((acct != string(tran.FromID)) && (acct != string(tran.ToID))) {
			continue
		}

		trans = append(trans, tx{
			FromAccount: tran.FromID,
			FromName:    h.NS.Lookup(tran.FromID),
			To:          tran.ToID,
			ToName:      h.NS.Lookup(tran.ToID),
			ChainID:     tran.ChainID,
			Nonce:       tran.Nonce,
			Value:       tran.Value,
			Tip:         tran.Tip,
			Data:        tran.Data,
			TimeStamp:   tran.TimeStamp,
			GasPrice:    tran.GasPrice,
			GasUnits:    tran.GasUnits,
			// Sig:         tran.SignatureString(),
		})
	}

	return web.Respond(ctx, w, trans, http.StatusOK)
}

// Accounts returns the current balances for all users.
func (h Handlers) Accounts(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	accountStr := web.Param(r, "account")

	var accounts map[database.AccountID]database.Account
	switch accountStr {
	case "":
		accounts = h.State.RetriveAccounts()

	default:
		accountID, err := database.ToAccountID(accountStr)
		if err != nil {
			return err
		}
		account, err := h.State.QueryAccounts(accountID)
		if err != nil {
			return err
		}
		accounts = map[database.AccountID]database.Account{accountID: account}
	}

	resp := make([]act, 0, len(accounts))
	for account, info := range accounts {
		act := act{
			Account: account,
			Name:    h.NS.Lookup(account),
			Balance: info.Balance,
			Nonce:   info.Nonce,
		}
		resp = append(resp, act)
	}

	ai := actInfo{
		LastestBlock: h.State.RetriveALatestBlock().Hash(),
		Uncommitted:  len(h.State.RetriveMempool()),
		Accounts:     resp,
	}

	return web.Respond(ctx, w, ai, http.StatusOK)
}
