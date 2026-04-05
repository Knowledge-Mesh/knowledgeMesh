package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func (s *PostgresStore) initBuyerBilling(ctx context.Context, buyerID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO buyer_billing (buyer_id, wallet_balance, token_quota, tokens_used)
		VALUES ($1, 1000000, 10000000, 0)
		ON CONFLICT (buyer_id) DO NOTHING`, buyerID)
	return err
}

func (s *PostgresStore) initSellerBilling(ctx context.Context, sellerID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO seller_billing (seller_id, wallet_balance, token_quota, tokens_used)
		VALUES ($1, 0, 10000000, 0)
		ON CONFLICT (seller_id) DO NOTHING`, sellerID)
	return err
}

// BuyerBillingSnapshot is persisted wallet / quota / usage for a buyer.
type BuyerBillingSnapshot struct {
	WalletBalance int64 `json:"walletBalance"`
	TokenQuota    int64 `json:"tokenQuota"`
	TokensUsed    int64 `json:"tokensUsed"`
}

// SellerBillingSnapshot is persisted wallet / quota / usage for a seller.
type SellerBillingSnapshot struct {
	WalletBalance int64 `json:"walletBalance"`
	TokenQuota    int64 `json:"tokenQuota"`
	TokensUsed    int64 `json:"tokensUsed"`
}

func (s *PostgresStore) GetBuyerBillingSnapshot(buyerID string) (BuyerBillingSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var snap BuyerBillingSnapshot
	err := s.pool.QueryRow(ctx, `
		SELECT wallet_balance, token_quota, tokens_used FROM buyer_billing WHERE buyer_id = $1::uuid`, buyerID).
		Scan(&snap.WalletBalance, &snap.TokenQuota, &snap.TokensUsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BuyerBillingSnapshot{}, errors.New("buyer billing not found")
		}
		return BuyerBillingSnapshot{}, err
	}
	return snap, nil
}

func (s *PostgresStore) GetSellerBillingSnapshot(sellerID string) (SellerBillingSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var snap SellerBillingSnapshot
	err := s.pool.QueryRow(ctx, `
		SELECT wallet_balance, token_quota, tokens_used FROM seller_billing WHERE seller_id = $1::uuid`, sellerID).
		Scan(&snap.WalletBalance, &snap.TokenQuota, &snap.TokensUsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SellerBillingSnapshot{}, errors.New("seller billing not found")
		}
		return SellerBillingSnapshot{}, err
	}
	return snap, nil
}

func estimateTokensForMatch(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 1
	}
	return len(strings.Fields(v))
}

func buildSellerNodeForMatch(peerID string, onDuty bool, models []SellerModelRecord, listenAddrs []string) types.SellerNode {
	var skills []types.Skill
	for _, m := range models {
		if !m.Active {
			continue
		}
		skills = append(skills, types.Skill{
			Name:       m.SkillName,
			ModelName:  m.ModelName,
			ModelType:  m.ModelType,
			TuningTier: m.TuningTier,
			Price:      m.RatePerToken,
		})
	}
	modelName, modelType, tuning, price := "", "", "", 0.0
	if len(skills) > 0 {
		modelName = skills[0].ModelName
		modelType = skills[0].ModelType
		tuning = skills[0].TuningTier
		price = skills[0].Price
	}
	limits := types.RateLimits{}
	if len(models) > 0 {
		m := models[0]
		limits.HourlyTokens = m.HourlyTokens
		limits.DailyTokens = m.DailyTokens
		limits.TotalTokens = m.TotalTokens
	}
	return types.SellerNode{
		PeerID:      peerID,
		ListenAddrs: listenAddrs,
		Skills:      skills,
		ModelName:   modelName,
		ModelType:   modelType,
		TuningTier:  tuning,
		Price:       price,
		Reputation:  1,
		OnDuty:      onDuty,
		TokenLimits: limits,
	}
}

// FindSellerIDByPeerID returns the seller account id for a reported libp2p host id.
func (s *PostgresStore) FindSellerIDByPeerID(peerID string) (string, error) {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return "", errors.New("empty peer id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM seller_users WHERE peer_id = $1`, peerID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSellerNotFound
		}
		return "", err
	}
	return id.String(), nil
}

// ListSellerNodesForMatch returns on-duty sellers with a libp2p peer id and at least one active model skill.
func (s *PostgresStore) ListSellerNodesForMatch() ([]types.SellerNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, peer_id, on_duty, COALESCE(listen_addrs, '[]'::jsonb) FROM seller_users
		WHERE status = 'active' AND on_duty = true
		  AND peer_id IS NOT NULL AND trim(peer_id) <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.SellerNode
	for rows.Next() {
		var sid, peer string
		var onDuty bool
		var listenJSON []byte
		if err := rows.Scan(&sid, &peer, &onDuty, &listenJSON); err != nil {
			return nil, err
		}
		var listenAddrs []string
		if len(listenJSON) > 0 {
			_ = json.Unmarshal(listenJSON, &listenAddrs)
		}
		models, err := s.loadSellerModels(ctx, sid)
		if err != nil {
			return nil, err
		}
		node := buildSellerNodeForMatch(peer, onDuty, models, listenAddrs)
		if len(node.Skills) == 0 {
			continue
		}
		out = append(out, node)
	}
	return out, rows.Err()
}

// InsertInferenceMatch records a control-plane match for later settlement and audit.
func (s *PostgresStore) InsertInferenceMatch(requestID, buyerID, sellerID, sellerPeerID string, estimatedTokens int) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("requestId is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO inference_matches (request_id, buyer_id, seller_id, seller_peer_id, estimated_tokens, settled)
		VALUES ($1, $2::uuid, $3::uuid, $4, $5, false)`,
		requestID, buyerID, sellerID, sellerPeerID, estimatedTokens)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateInferenceMatch
		}
		return err
	}
	return nil
}

// SettleInferenceMatch applies token movement between buyer and seller wallets (idempotent per request_id).
func (s *PostgresStore) SettleInferenceMatch(buyerID, requestID string, totalTokens int64, success bool) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("requestId is required")
	}
	if totalTokens < 0 {
		totalTokens = 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sellerID uuid.UUID
	var settled bool
	err = tx.QueryRow(ctx, `
		SELECT seller_id, settled FROM inference_matches WHERE request_id = $1 AND buyer_id = $2::uuid`,
		requestID, buyerID).Scan(&sellerID, &settled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInferenceMatchNotFound
		}
		return err
	}
	if settled {
		return tx.Commit(ctx)
	}
	if !success || totalTokens == 0 {
		_, err = tx.Exec(ctx, `UPDATE inference_matches SET settled = true WHERE request_id = $1`, requestID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	var bWallet, bQuota, bUsed int64
	err = tx.QueryRow(ctx, `SELECT wallet_balance, token_quota, tokens_used FROM buyer_billing WHERE buyer_id = $1::uuid FOR UPDATE`, buyerID).
		Scan(&bWallet, &bQuota, &bUsed)
	if err != nil {
		return err
	}
	if bUsed+totalTokens > bQuota {
		return ErrInferenceQuotaExceeded
	}
	if bWallet < totalTokens {
		return ErrInsufficientBalance
	}

	var sWallet, sQuota, sUsed int64
	err = tx.QueryRow(ctx, `SELECT wallet_balance, token_quota, tokens_used FROM seller_billing WHERE seller_id = $1 FOR UPDATE`, sellerID).
		Scan(&sWallet, &sQuota, &sUsed)
	if err != nil {
		return err
	}

	newBWallet := bWallet - totalTokens
	newBUsed := bUsed + totalTokens
	newSWallet := sWallet + totalTokens
	newSUsed := sUsed + totalTokens

	_, err = tx.Exec(ctx, `
		UPDATE buyer_billing SET wallet_balance = $2, tokens_used = $3, updated_at = now() WHERE buyer_id = $1::uuid`,
		buyerID, newBWallet, newBUsed)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE seller_billing SET wallet_balance = $2, tokens_used = $3, updated_at = now() WHERE seller_id = $1`,
		sellerID, newSWallet, newSUsed)
	if err != nil {
		return err
	}

	tid1 := uuid.New()
	tid2 := uuid.New()
	details := map[string]any{"requestId": requestID, "totalTokens": totalTokens}
	d1, _ := json.Marshal(details)
	d2, _ := json.Marshal(details)

	_, err = tx.Exec(ctx, `
		INSERT INTO billing_transactions (id, party_kind, party_id, tx_type, amount, balance_after, details)
		VALUES ($1, 'buyer', $2::uuid, $3, $4, $5, $6)`,
		tid1, buyerID, TxBuyerInferenceDebit, -totalTokens, newBWallet, d1)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO billing_transactions (id, party_kind, party_id, tx_type, amount, balance_after, details)
		VALUES ($1, 'seller', $2, $3, $4, $5, $6)`,
		tid2, sellerID, TxSellerInferenceCredit, totalTokens, newSWallet, d2)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE inference_matches SET settled = true WHERE request_id = $1`, requestID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AppendBuyerInferenceTracking logs a non-settlement audit entry (amount 0).
func (s *PostgresStore) AppendBuyerInferenceTracking(buyerID, requestID, phase string, meta map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snap, err := s.GetBuyerBillingSnapshot(buyerID)
	if err != nil {
		return err
	}
	details := map[string]any{"requestId": requestID, "phase": phase}
	for k, v := range meta {
		details[k] = v
	}
	b, _ := json.Marshal(details)
	id := uuid.New()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO billing_transactions (id, party_kind, party_id, tx_type, amount, balance_after, details)
		VALUES ($1, 'buyer', $2::uuid, $3, 0, $4, $5)`,
		id, buyerID, TxBuyerInferenceTrack, snap.WalletBalance, b)
	return err
}

// GetInferenceMatchSeller returns the seller id recorded for a match (for seller tracking validation).
func (s *PostgresStore) GetInferenceMatchSeller(requestID string) (sellerID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var sid uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT seller_id FROM inference_matches WHERE request_id = $1`, requestID).Scan(&sid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrInferenceMatchNotFound
		}
		return "", err
	}
	return sid.String(), nil
}

// AppendSellerInferenceTracking logs seller-reported execution metadata (amount 0).
func (s *PostgresStore) AppendSellerInferenceTracking(sellerID, requestID string, totalTokens int64, success bool, meta map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snap, err := s.GetSellerBillingSnapshot(sellerID)
	if err != nil {
		return err
	}
	details := map[string]any{"requestId": requestID, "totalTokens": totalTokens, "success": success}
	for k, v := range meta {
		details[k] = v
	}
	b, _ := json.Marshal(details)
	id := uuid.New()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO billing_transactions (id, party_kind, party_id, tx_type, amount, balance_after, details)
		VALUES ($1, 'seller', $2::uuid, $3, 0, $4, $5)`,
		id, sellerID, TxSellerInferenceTrack, snap.WalletBalance, b)
	return err
}

// CheckBuyerCanSpendTokens validates wallet and quota against an estimated spend (pre-match).
func (s *PostgresStore) CheckBuyerCanSpendTokens(buyerID string, estimatedTokens int64) error {
	snap, err := s.GetBuyerBillingSnapshot(buyerID)
	if err != nil {
		return err
	}
	if snap.TokensUsed+estimatedTokens > snap.TokenQuota {
		return ErrInferenceQuotaExceeded
	}
	if snap.WalletBalance < estimatedTokens {
		return ErrInsufficientBalance
	}
	return nil
}
