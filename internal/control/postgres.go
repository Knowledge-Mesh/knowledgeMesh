package control

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrBuyerExists              = errors.New("buyer already exists")
	ErrSellerExists             = errors.New("seller already exists")
	ErrSellerNotFound           = errors.New("seller not found")
	ErrModelNotFound            = errors.New("model not found")
	ErrInvalidLogin             = errors.New("invalid credentials")
	ErrInsufficientBalance      = errors.New("insufficient token balance")
	ErrInferenceQuotaExceeded   = errors.New("token quota exceeded")
	ErrInferenceMatchNotFound   = errors.New("inference match not found")
	ErrDuplicateInferenceMatch  = errors.New("duplicate inference request id")
)

// PostgresStore persists buyer_users in PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore opens a connection pool.
func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

// Pool returns the underlying pool (for graceful shutdown in tests).
func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

// Migrate applies buyer and seller schemas.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, BuyerUsersSQL); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, SellerSchemaSQL); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, BillingSchemaSQL); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, BillingBackfillSQL)
	return err
}

// RegisterBuyer inserts a new buyer row (email must be unique, case-insensitive).
func (s *PostgresStore) RegisterBuyer(displayName, email, password string) (string, error) {
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
		INSERT INTO buyer_users (id, display_name, email, password_hash, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'active', $5, $5)`,
		id, displayName, email, string(hash), now)
	if err != nil {
		if isUniqueViolation(err) {
			return "", ErrBuyerExists
		}
		return "", err
	}
	if err := s.initBuyerBilling(ctx, id); err != nil {
		return "", err
	}
	return id.String(), nil
}

// LoginBuyer verifies credentials and returns id, display name, and canonical email.
func (s *PostgresStore) LoginBuyer(email, password string) (buyerID, displayName, em string, err error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return "", "", "", ErrInvalidLogin
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var id uuid.UUID
	var name, hash string
	var status string
	err = s.pool.QueryRow(ctx, `
		SELECT id, display_name, email, password_hash, status FROM buyer_users WHERE lower(email) = lower($1)`, email).
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
	_, _ = s.pool.Exec(ctx, `UPDATE buyer_users SET updated_at = now() WHERE id = $1`, id)
	return id.String(), name, em, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
