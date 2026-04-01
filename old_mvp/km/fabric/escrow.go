package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

type EscrowHold struct {
	TaskID string
	Buyer  string
	Amount float64
}

type Escrow struct {
	mu    sync.Mutex
	holds map[string]*EscrowHold // keyed by task_id
}

func NewEscrow() *Escrow {
	return &Escrow{
		holds: make(map[string]*EscrowHold),
	}
}

// Lock checks the buyer's balance and locks the escrow amount atomically.
func (e *Escrow) Lock(ledger *Ledger, taskID, buyer string, amount float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Atomically check balance and debit under the ledger's lock
	_, err := ledger.BalanceAndDebit(buyer, amount, "escrow_lock", fmt.Sprintf("%s escrow locked", taskID))
	if err != nil {
		errMsg := err.Error()
		if strings.HasPrefix(errMsg, "no_credits:") {
			return fmt.Errorf("You have no credits remaining. Earn credits by selling compute, or contact admin for a top-up.")
		}
		if strings.HasPrefix(errMsg, "insufficient_credits:") {
			bal, _ := ledger.Balance(buyer)
			return fmt.Errorf("Insufficient credits. You have $%.4f remaining but this request requires ~$%.4f escrow. Earn credits by selling compute.", bal, amount)
		}
		return err
	}

	e.holds[taskID] = &EscrowHold{
		TaskID: taskID,
		Buyer:  buyer,
		Amount: amount,
	}

	return nil
}

// Settle charges the actual cost and refunds the overage.
func (e *Escrow) Settle(ledger *Ledger, taskID, seller string, actualTokens int, pricePerM float64) (charged, refunded float64, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	hold, ok := e.holds[taskID]
	if !ok {
		return 0, 0, fmt.Errorf("no escrow hold for task %s", taskID)
	}

	actualCost := float64(actualTokens) / 1_000_000.0 * pricePerM
	if actualCost > hold.Amount {
		actualCost = hold.Amount // cap at escrow amount
	}

	refund := hold.Amount - actualCost

	detail := fmt.Sprintf("%s settled %d tokens @ $%.2f/M", taskID, actualTokens, pricePerM)

	// Credit seller
	if err := ledger.Append("escrow_settle", seller, actualCost, detail); err != nil {
		return 0, 0, fmt.Errorf("write seller credit: %w", err)
	}

	// Refund buyer overage
	if refund > 0.001 { // avoid tiny refund entries
		if err := ledger.Append("escrow_refund", hold.Buyer, refund, fmt.Sprintf("%s overage returned", taskID)); err != nil {
			return 0, 0, fmt.Errorf("write buyer refund: %w", err)
		}
	}

	delete(e.holds, taskID)
	return actualCost, refund, nil
}

// Release returns the full escrow amount on failure.
func (e *Escrow) Release(ledger *Ledger, taskID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	hold, ok := e.holds[taskID]
	if !ok {
		return nil // nothing to release
	}

	err := ledger.Append("escrow_release", hold.Buyer, hold.Amount, fmt.Sprintf("%s released — task failed", taskID))
	delete(e.holds, taskID)
	return err
}

func (e *Escrow) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.holds = make(map[string]*EscrowHold)
	log.Printf("[escrow] Reset — all holds cleared")
}
