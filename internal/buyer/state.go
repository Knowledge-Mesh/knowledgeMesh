package buyer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

var (
	ErrBuyerExists         = errors.New("buyer already exists")
	ErrBuyerNotFound       = errors.New("buyer not found")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidSession      = errors.New("invalid session")
	ErrHourlyLimitExceeded = errors.New("hourly token limit exceeded")
	ErrDailyLimitExceeded  = errors.New("daily token limit exceeded")
	ErrTotalLimitExceeded  = errors.New("total token limit exceeded")
)

type Preferences struct {
	PreferredSkill     string  `json:"preferredSkill"`
	PreferredModel     string  `json:"preferredModel"`
	MaxPricePerToken   float64 `json:"maxPricePerToken"`
	MaxPricePerRequest float64 `json:"maxPricePerRequest"`
}

type RequestRecord struct {
	RequestID   string    `json:"requestId"`
	Prompt      string    `json:"prompt"`
	Tokens      int       `json:"tokens"`
	Price       float64   `json:"price"`
	RequestedAt time.Time `json:"requestedAt"`
}

type State struct {
	BuyerID        string              `json:"buyerId"`
	SessionID      string              `json:"sessionId"`
	AuthRef        string              `json:"authRef"`
	TokenLimits    types.RateLimits    `json:"tokenLimits"`
	Usage          types.UsageCounters `json:"usage"`
	Preferences    Preferences         `json:"preferences"`
	RequestHistory []RequestRecord     `json:"requestHistory"`
}

type buyerAccount struct {
	Username     string
	Email        string
	PasswordHash string
	State        State
}

type stateStore interface {
	Create(account buyerAccount) error
	GetByUserOrEmail(user string) (buyerAccount, bool)
	GetBySession(sessionID string) (buyerAccount, bool)
	Update(account buyerAccount) error
}

type inMemoryStore struct {
	mu         sync.Mutex
	byUsername map[string]buyerAccount
	byEmail    map[string]string
	bySession  map[string]string
}

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		byUsername: map[string]buyerAccount{},
		byEmail:    map[string]string{},
		bySession:  map[string]string{},
	}
}

func (s *inMemoryStore) Create(account buyerAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userKey := strings.ToLower(account.Username)
	emailKey := strings.ToLower(account.Email)
	if _, ok := s.byUsername[userKey]; ok {
		return ErrBuyerExists
	}
	if _, ok := s.byEmail[emailKey]; ok {
		return ErrBuyerExists
	}
	s.byUsername[userKey] = account
	s.byEmail[emailKey] = userKey
	if account.State.SessionID != "" {
		s.bySession[account.State.SessionID] = userKey
	}
	return nil
}

func (s *inMemoryStore) GetByUserOrEmail(user string) (buyerAccount, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(user))
	if account, ok := s.byUsername[key]; ok {
		return account, true
	}
	if username, ok := s.byEmail[key]; ok {
		account, exists := s.byUsername[username]
		return account, exists
	}
	return buyerAccount{}, false
}

func (s *inMemoryStore) GetBySession(sessionID string) (buyerAccount, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	username, ok := s.bySession[sessionID]
	if !ok {
		return buyerAccount{}, false
	}
	account, exists := s.byUsername[username]
	return account, exists
}

func (s *inMemoryStore) Update(account buyerAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(account.Username)
	if _, ok := s.byUsername[key]; !ok {
		return ErrBuyerNotFound
	}
	s.byUsername[key] = account
	if account.State.SessionID != "" {
		s.bySession[account.State.SessionID] = key
	}
	return nil
}

type Manager struct {
	store stateStore
}

func NewManager() *Manager {
	return &Manager{store: newInMemoryStore()}
}

func (m *Manager) Register(email, username, password string) (State, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	username = strings.TrimSpace(username)
	if email == "" || username == "" || password == "" {
		return State{}, ErrInvalidCredentials
	}

	buyerID := "buyer-" + username
	account := buyerAccount{
		Username:     username,
		Email:        email,
		PasswordHash: hash(password),
		State: State{
			BuyerID:     buyerID,
			AuthRef:     "local:" + email,
			TokenLimits: types.RateLimits{},
			Usage:       types.UsageCounters{},
		},
	}
	if err := m.store.Create(account); err != nil {
		return State{}, err
	}
	return account.State, nil
}

func (m *Manager) Login(userOrEmail, password string) (State, error) {
	account, ok := m.store.GetByUserOrEmail(userOrEmail)
	if !ok {
		return State{}, ErrInvalidCredentials
	}
	if account.PasswordHash != hash(password) {
		return State{}, ErrInvalidCredentials
	}

	account.State.SessionID = fmt.Sprintf("sess-%d", time.Now().UTC().UnixNano())
	if err := m.store.Update(account); err != nil {
		return State{}, err
	}
	return account.State, nil
}

func (m *Manager) UpdatePreferences(sessionID string, prefs Preferences) (State, error) {
	account, err := m.getAccountBySession(sessionID)
	if err != nil {
		return State{}, err
	}
	account.State.Preferences = prefs
	if err := m.store.Update(account); err != nil {
		return State{}, err
	}
	return account.State, nil
}

func (m *Manager) SetLimits(sessionID string, hourly, daily, total int) (State, error) {
	account, err := m.getAccountBySession(sessionID)
	if err != nil {
		return State{}, err
	}
	account.State.TokenLimits.HourlyTokens = hourly
	account.State.TokenLimits.DailyTokens = daily
	account.State.TokenLimits.TotalTokens = total
	if err := m.store.Update(account); err != nil {
		return State{}, err
	}
	return account.State, nil
}

func (m *Manager) CheckLimits(sessionID string, tokens int, now time.Time) error {
	account, err := m.getAccountBySession(sessionID)
	if err != nil {
		return err
	}
	usage := account.State.Usage
	usage = resetIfBucketChanged(usage, now)
	limits := account.State.TokenLimits

	if limits.HourlyTokens > 0 && usage.HourlyTokens+tokens > limits.HourlyTokens {
		return ErrHourlyLimitExceeded
	}
	if limits.DailyTokens > 0 && usage.DailyTokens+tokens > limits.DailyTokens {
		return ErrDailyLimitExceeded
	}
	if limits.TotalTokens > 0 && usage.TotalTokens+tokens > limits.TotalTokens {
		return ErrTotalLimitExceeded
	}
	return nil
}

func (m *Manager) ConsumeUsage(sessionID string, requestID, prompt string, tokens int, price float64, now time.Time) (State, error) {
	account, err := m.getAccountBySession(sessionID)
	if err != nil {
		return State{}, err
	}
	if err := m.CheckLimits(sessionID, tokens, now); err != nil {
		return State{}, err
	}

	usage := resetIfBucketChanged(account.State.Usage, now)
	usage.RequestsServed++
	usage.TotalTokens += tokens
	usage.HourlyTokens += tokens
	usage.DailyTokens += tokens
	account.State.Usage = usage
	account.State.RequestHistory = append(account.State.RequestHistory, RequestRecord{
		RequestID:   requestID,
		Prompt:      prompt,
		Tokens:      tokens,
		Price:       price,
		RequestedAt: now.UTC(),
	})

	if err := m.store.Update(account); err != nil {
		return State{}, err
	}
	return account.State, nil
}

func (m *Manager) ResetHourly(sessionID string, now time.Time) (State, error) {
	account, err := m.getAccountBySession(sessionID)
	if err != nil {
		return State{}, err
	}
	account.State.Usage.HourlyTokens = 0
	account.State.Usage.HourBucketUnix = now.UTC().Truncate(time.Hour).Unix()
	if err := m.store.Update(account); err != nil {
		return State{}, err
	}
	return account.State, nil
}

func (m *Manager) ResetDaily(sessionID string, now time.Time) (State, error) {
	account, err := m.getAccountBySession(sessionID)
	if err != nil {
		return State{}, err
	}
	account.State.Usage.DailyTokens = 0
	account.State.Usage.DayBucketUnix = dayBucket(now)
	if err := m.store.Update(account); err != nil {
		return State{}, err
	}
	return account.State, nil
}

func (m *Manager) SubmitPrompt(sessionID, requestID, prompt string, tokens int, price float64, now time.Time) (RequestRecord, error) {
	state, err := m.ConsumeUsage(sessionID, requestID, prompt, tokens, price, now)
	if err != nil {
		return RequestRecord{}, err
	}
	return state.RequestHistory[len(state.RequestHistory)-1], nil
}

func (m *Manager) getAccountBySession(sessionID string) (buyerAccount, error) {
	account, ok := m.store.GetBySession(sessionID)
	if !ok {
		return buyerAccount{}, ErrInvalidSession
	}
	return account, nil
}

func resetIfBucketChanged(usage types.UsageCounters, now time.Time) types.UsageCounters {
	hour := now.UTC().Truncate(time.Hour).Unix()
	day := dayBucket(now)
	if usage.HourBucketUnix != hour {
		usage.HourlyTokens = 0
		usage.HourBucketUnix = hour
	}
	if usage.DayBucketUnix != day {
		usage.DailyTokens = 0
		usage.DayBucketUnix = day
	}
	return usage
}

func dayBucket(now time.Time) int64 {
	n := now.UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC).Unix()
}

func hash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}
