package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// SellerProfile is a seller row plus models (for API / DB round-trip).
type SellerProfile struct {
	SellerID  string              `json:"sellerId"`
	Name      string              `json:"name"`
	Email     string              `json:"email"`
	OnDuty    bool                `json:"onDuty"`
	PeerID    string              `json:"peerId,omitempty"`
	Models    []SellerModelRecord `json:"models"`
	CreatedAt time.Time           `json:"createdAt,omitempty"`
}

// RegisterSeller inserts seller_users only (models added via ReplaceSellerModels).
func (s *PostgresStore) RegisterSeller(displayName, email, password string) (string, error) {
	displayName = strings.TrimSpace(displayName)
	email = strings.TrimSpace(strings.ToLower(email))
	if displayName == "" || email == "" || password == "" {
		return "", errors.New("name, email, and password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	id := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	now := time.Now().UTC()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO seller_users (id, display_name, email, password_hash, on_duty, created_at, updated_at)
		VALUES ($1, $2, $3, $4, false, $5, $5)`,
		id, displayName, email, string(hash), now)
	if err != nil {
		if isUniqueViolation(err) {
			return "", ErrSellerExists
		}
		return "", err
	}
	if err := s.initSellerBilling(ctx, id); err != nil {
		return "", err
	}
	return id.String(), nil
}

// LoginSeller verifies credentials and returns id, display name, email.
func (s *PostgresStore) LoginSeller(email, password string) (sellerID, displayName, em string, err error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return "", "", "", ErrInvalidLogin
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var id uuid.UUID
	var name, hash, status string
	err = s.pool.QueryRow(ctx, `
		SELECT id, display_name, email, password_hash, status FROM seller_users WHERE lower(email) = lower($1)`, email).
		Scan(&id, &name, &em, &hash, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", ErrInvalidLogin
		}
		return "", "", "", err
	}
	if status != "active" {
		return "", "", "", ErrInvalidLogin
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", "", "", ErrInvalidLogin
	}
	_, _ = s.pool.Exec(ctx, `UPDATE seller_users SET updated_at = now() WHERE id = $1`, id)
	return id.String(), name, em, nil
}

// GetSellerProfile loads seller + models.
func (s *PostgresStore) GetSellerProfile(sellerID string) (SellerProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var id uuid.UUID
	var name, em string
	var onDuty bool
	var peerID *string
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, display_name, email, on_duty, peer_id, created_at FROM seller_users WHERE id = $1::uuid`, sellerID).
		Scan(&id, &name, &em, &onDuty, &peerID, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SellerProfile{}, ErrSellerNotFound
		}
		return SellerProfile{}, err
	}
	models, err := s.loadSellerModels(ctx, sellerID)
	if err != nil {
		return SellerProfile{}, err
	}
	p := ""
	if peerID != nil {
		p = *peerID
	}
	return SellerProfile{
		SellerID:  id.String(),
		Name:      name,
		Email:     em,
		OnDuty:    onDuty,
		PeerID:    p,
		Models:    models,
		CreatedAt: createdAt,
	}, nil
}

func (s *PostgresStore) loadSellerModels(ctx context.Context, sellerID string) ([]SellerModelRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, COALESCE(version,''), skill_name, model_name, COALESCE(model_type,''), COALESCE(tuning_tier,''),
			hourly_tokens, daily_tokens, total_tokens, rate_per_token, active, meta
		FROM seller_models WHERE seller_id = $1::uuid ORDER BY name`, sellerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SellerModelRecord
	for rows.Next() {
		var m SellerModelRecord
		var metaBytes []byte
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Version, &m.SkillName, &m.ModelName, &m.ModelType, &m.TuningTier,
			&m.HourlyTokens, &m.DailyTokens, &m.TotalTokens, &m.RatePerToken, &m.Active, &metaBytes,
		); err != nil {
			return nil, err
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &m.Meta)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceSellerModels replaces all models for a seller.
func (s *PostgresStore) ReplaceSellerModels(sellerID string, models []SellerModelRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var n int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM seller_users WHERE id = $1::uuid`, sellerID).Scan(&n); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSellerNotFound
		}
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM seller_models WHERE seller_id = $1::uuid`, sellerID)
	if err != nil {
		return err
	}
	for i := range models {
		if models[i].ID == "" {
			models[i].ID = uuid.New().String()
		}
		metaJSON, _ := json.Marshal(models[i].Meta)
		_, err = tx.Exec(ctx, `
			INSERT INTO seller_models (
				id, seller_id, name, version, skill_name, model_name, model_type, tuning_tier,
				hourly_tokens, daily_tokens, total_tokens, rate_per_token, active, meta
			) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			models[i].ID, sellerID, models[i].Name, nullStr(models[i].Version), models[i].SkillName,
			models[i].ModelName, models[i].ModelType, models[i].TuningTier,
			models[i].HourlyTokens, models[i].DailyTokens, models[i].TotalTokens, models[i].RatePerToken,
			models[i].Active, metaJSON,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// SetSellerDuty updates on_duty.
func (s *PostgresStore) SetSellerDuty(sellerID string, onDuty bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd, err := s.pool.Exec(ctx, `UPDATE seller_users SET on_duty = $2, updated_at = now() WHERE id = $1::uuid`, sellerID, onDuty)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrSellerNotFound
	}
	return nil
}

// SetSellerPeer updates last reported libp2p peer id (often same as host id string).
func (s *PostgresStore) SetSellerPeer(sellerID, peerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p := strings.TrimSpace(peerID)
	var pval any
	if p == "" {
		pval = nil
	} else {
		pval = p
	}
	cmd, err := s.pool.Exec(ctx, `UPDATE seller_users SET peer_id = $2, updated_at = now() WHERE id = $1::uuid`, sellerID, pval)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrSellerNotFound
	}
	return nil
}

// SetModelActive toggles one model's active flag.
func (s *PostgresStore) SetModelActive(sellerID, modelID string, active bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd, err := s.pool.Exec(ctx, `
		UPDATE seller_models SET active = $3 WHERE seller_id = $1::uuid AND id = $2::uuid`,
		sellerID, modelID, active)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrModelNotFound
	}
	return nil
}
