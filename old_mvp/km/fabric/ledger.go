package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type Ledger struct {
	mu               sync.Mutex
	filepath         string
	balances         map[string]float64 // in-memory cache: user → balance
	users            map[string]bool    // in-memory cache: known users
	taskCount        int                // in-memory cache: escrow_settle count (Amount > 0)
	totalApiCost     float64            // in-memory cache: total API cost from settle events
	totalKmCost      float64            // in-memory cache: total KM cost from settle events
	totalTokensTraded int64             // in-memory cache: total tokens traded from settle events
}

func NewLedger(filepath string) *Ledger {
	l := &Ledger{
		filepath: filepath,
		balances: make(map[string]float64),
		users:    make(map[string]bool),
	}
	l.loadCaches()
	return l
}

// loadCaches reads the entire ledger file once at startup to populate in-memory caches.
func (l *Ledger) loadCaches() {
	entries, err := l.readAll()
	if err != nil {
		log.Printf("[ledger] Warning: failed to load caches from file: %v", err)
		return
	}
	for _, e := range entries {
		l.balances[e.User] += e.Amount
		l.users[e.User] = true
		if e.Event == "escrow_settle" && e.Amount > 0 {
			l.taskCount++
			l.parseSettleDetail(e.Detail)
		}
	}
	log.Printf("[ledger] Loaded caches: %d users, %d tasks, %d tokens traded, %d entries", len(l.users), l.taskCount, l.totalTokensTraded, len(entries))
}

// updateCaches updates the in-memory caches after a new entry is appended.
// Caller must hold l.mu.
func (l *Ledger) updateCaches(event, user string, amount float64, detail string) {
	l.balances[user] += amount
	l.users[user] = true
	if event == "escrow_settle" && amount > 0 {
		l.taskCount++
		l.parseSettleDetail(detail)
	}
}

// parseSettleDetail extracts tokens and price from a settle detail string
// like "task-xxx settled N tokens @ $P/M" and updates cached totals.
func (l *Ledger) parseSettleDetail(detail string) {
	var taskID string
	var tokens int64
	var price float64
	n, _ := fmt.Sscanf(detail, "%s settled %d tokens @ $%f/M", &taskID, &tokens, &price)
	if n >= 2 {
		l.totalTokensTraded += tokens
		l.totalKmCost += float64(tokens) * price / 1_000_000
	}
}

// Append writes a single ledger entry as a JSON line.
func (l *Ledger) Append(event, user string, amount float64, detail string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.appendUnlocked(event, user, amount, detail)
}

// appendUnlocked writes a ledger entry without acquiring the mutex.
// The caller must already hold l.mu.
func (l *Ledger) appendUnlocked(event, user string, amount float64, detail string) error {
	entry := LedgerEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Event:     event,
		User:      user,
		Amount:    amount,
		Detail:    detail,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal ledger entry: %w", err)
	}

	f, err := os.OpenFile(l.filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open ledger file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(string(data) + "\n")
	if err != nil {
		return err
	}

	// Update in-memory caches
	l.updateCaches(event, user, amount, detail)
	return nil
}

// Balance returns a user's current balance from the in-memory cache.
func (l *Ledger) Balance(user string) (float64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.balances[user], nil
}

func (l *Ledger) balanceUnlocked(user string) (float64, error) {
	return l.balances[user], nil
}

// Balances returns all user balances from the in-memory cache.
func (l *Ledger) Balances() (map[string]float64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Return a copy to avoid data races
	result := make(map[string]float64, len(l.balances))
	for k, v := range l.balances {
		result[k] = v
	}
	return result, nil
}

// TaskCount returns the number of completed tasks from the in-memory cache.
func (l *Ledger) TaskCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.taskCount
}

// BalanceAndDebit atomically checks the user's balance and debits the amount
// under a single lock, preventing race conditions between balance check and debit.
func (l *Ledger) BalanceAndDebit(buyer string, amount float64, event, detail string) (float64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	balance := l.balances[buyer]
	if balance < amount {
		if balance <= 0.001 {
			return balance, fmt.Errorf("no_credits: balance is zero")
		}
		return balance, fmt.Errorf("insufficient_credits: have %.4f, need %.4f", balance, amount)
	}

	err := l.appendUnlocked(event, buyer, -amount, detail)
	if err != nil {
		return balance, fmt.Errorf("write debit: %w", err)
	}

	return balance, nil
}

// EnsureUser creates an initial credit entry if the user has no entries.
func (l *Ledger) EnsureUser(user string, initial float64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.users[user] {
		return nil // user already exists
	}

	err := l.appendUnlocked("initial_credit", user, initial, "genesis")
	if err == nil {
		log.Printf("[ledger] Seeded %s with %.0f credits", user, initial)
	}
	return err
}

// UserExists checks if a user has any ledger entries using the in-memory cache.
func (l *Ledger) UserExists(user string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.users[user]
}

// Reset clears the ledger file and resets all in-memory caches.
func (l *Ledger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	os.Truncate(l.filepath, 0)
	l.balances = make(map[string]float64)
	l.users = make(map[string]bool)
	l.taskCount = 0
	l.totalApiCost = 0
	l.totalKmCost = 0
	l.totalTokensTraded = 0
	log.Printf("[ledger] Reset — ledger file cleared")
}

// RecentEntries returns the last N ledger entries (reads from disk).
func (l *Ledger) RecentEntries(n int) []LedgerEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, _ := l.readAll()
	if len(entries) <= n {
		return entries
	}
	return entries[len(entries)-n:]
}

// TokenStats returns total tokens traded and total KM cost from settle events (from cache).
func (l *Ledger) TokenStats() (int64, float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.totalTokensTraded, l.totalKmCost
}

func (l *Ledger) readAll() ([]LedgerEntry, error) {
	f, err := os.Open(l.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []LedgerEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e LedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}
