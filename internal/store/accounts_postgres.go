package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolation = "23505"

func isUnique(err error) bool {
	var pg *pgconn.PgError

	return errors.As(err, &pg) && pg.Code == uniqueViolation
}

func (s *Postgres) CreateAccount(ctx context.Context, username, passwordHash string, codeHashes []string) (Account, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, false, fmt.Errorf("store: create account: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	a := Account{Username: username, PasswordHash: passwordHash}
	err = tx.QueryRow(ctx, `INSERT INTO accounts (username, password_hash) VALUES ($1, $2) ON CONFLICT (username) DO NOTHING RETURNING id`, username, passwordHash).Scan(&a.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("store: create account: %w", err)
	}
	for _, h := range codeHashes {
		if _, err := tx.Exec(ctx, `INSERT INTO account_recovery_codes (account_id, code_hash) VALUES ($1, $2) ON CONFLICT DO NOTHING`, a.ID, h); err != nil {
			return Account{}, false, fmt.Errorf("store: create account codes: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, false, fmt.Errorf("store: create account: %w", err)
	}

	return a, true, nil
}

const accountColumns = `a.id, a.username, a.password_hash, COALESCE(a.pro_email, ''), COALESCE(a.email, ''), a.email_verified_at IS NOT NULL`

func (s *Postgres) accountWhere(ctx context.Context, join, where string, args ...any) (Account, bool, error) {
	var a Account
	err := s.pool.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts a `+join+` WHERE `+where, args...).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.ProEmail, &a.Email, &a.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("store: account: %w", err)
	}

	return a, true, nil
}

func (s *Postgres) AccountByUsername(ctx context.Context, username string) (Account, bool, error) {
	return s.accountWhere(ctx, "", "a.username = $1", username)
}

func (s *Postgres) AccountByEmail(ctx context.Context, email string) (Account, bool, error) {
	return s.accountWhere(ctx, "", "a.email = $1 AND a.email_verified_at IS NOT NULL", email)
}

func (s *Postgres) SetAccountEmail(ctx context.Context, id int64, email string) (bool, error) {
	var v any
	if email != "" {
		v = email
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("store: set account email: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `UPDATE accounts SET email = $2, email_verified_at = NULL WHERE id = $1`, id, v); err != nil {
		return false, fmt.Errorf("store: set account email: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM account_email_tokens WHERE account_id = $1`, id); err != nil {
		return false, fmt.Errorf("store: set account email: %w", err)
	}

	return true, tx.Commit(ctx)
}

func (s *Postgres) VerifyAccountEmail(ctx context.Context, id int64, email string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE accounts SET email_verified_at = now() WHERE id = $1 AND email = $2`, id, email)
	if isUnique(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: verify account email: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) SaveEmailToken(ctx context.Context, hash string, accountID int64, purpose, email string, expires time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: save email token: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM account_email_tokens WHERE account_id = $1 AND purpose = $2`, accountID, purpose); err != nil {
		return fmt.Errorf("store: save email token: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO account_email_tokens (token_hash, account_id, purpose, email, expires_at) VALUES ($1, $2, $3, $4, $5)`, hash, accountID, purpose, email, expires); err != nil {
		return fmt.Errorf("store: save email token: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *Postgres) UseEmailToken(ctx context.Context, hash, purpose string, now time.Time) (Account, string, bool, error) {
	var a Account
	var email string
	err := s.pool.QueryRow(ctx, `
		WITH used AS (
			DELETE FROM account_email_tokens WHERE token_hash = $1 AND purpose = $2 AND expires_at > $3
			RETURNING account_id, email
		)
		SELECT `+accountColumns+`, used.email FROM used JOIN accounts a ON a.id = used.account_id WHERE a.email = used.email`,
		hash, purpose, now).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.ProEmail, &a.Email, &a.EmailVerified, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, "", false, nil
	}
	if err != nil {
		return Account{}, "", false, fmt.Errorf("store: use email token: %w", err)
	}

	return a, email, true, nil
}

func (s *Postgres) SetAccountPassword(ctx context.Context, id int64, passwordHash string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE accounts SET password_hash = $2 WHERE id = $1`, id, passwordHash); err != nil {
		return fmt.Errorf("store: set account password: %w", err)
	}

	return nil
}

func (s *Postgres) ReplaceRecoveryCodes(ctx context.Context, id int64, codeHashes []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: replace recovery codes: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM account_recovery_codes WHERE account_id = $1`, id); err != nil {
		return fmt.Errorf("store: replace recovery codes: %w", err)
	}
	for _, h := range codeHashes {
		if _, err := tx.Exec(ctx, `INSERT INTO account_recovery_codes (account_id, code_hash) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, h); err != nil {
			return fmt.Errorf("store: replace recovery codes: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (s *Postgres) UseRecoveryCode(ctx context.Context, id int64, codeHash string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE account_recovery_codes SET used_at = now() WHERE account_id = $1 AND code_hash = $2 AND used_at IS NULL`, id, codeHash)
	if err != nil {
		return false, fmt.Errorf("store: use recovery code: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) RecoveryCodesLeft(ctx context.Context, id int64) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM account_recovery_codes WHERE account_id = $1 AND used_at IS NULL`, id).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: recovery codes left: %w", err)
	}

	return n, nil
}

func (s *Postgres) SaveAccountSession(ctx context.Context, hash string, accountID int64, expires time.Time) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM account_sessions WHERE expires_at < now()`); err != nil {
		return fmt.Errorf("store: prune account sessions: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO account_sessions (token_hash, account_id, expires_at) VALUES ($1, $2, $3)`, hash, accountID, expires); err != nil {
		return fmt.Errorf("store: save account session: %w", err)
	}

	return nil
}

func (s *Postgres) AccountBySession(ctx context.Context, hash string, now time.Time) (Account, bool, error) {
	return s.accountWhere(ctx, "JOIN account_sessions s ON s.account_id = a.id", "s.token_hash = $1 AND s.expires_at > $2", hash, now)
}

func (s *Postgres) DeleteAccountSession(ctx context.Context, hash string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM account_sessions WHERE token_hash = $1`, hash); err != nil {
		return fmt.Errorf("store: delete account session: %w", err)
	}

	return nil
}

func (s *Postgres) DeleteAccountSessions(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM account_sessions WHERE account_id = $1`, id); err != nil {
		return fmt.Errorf("store: delete account sessions: %w", err)
	}

	return nil
}

func (s *Postgres) DeleteAccount(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, id); err != nil {
		return fmt.Errorf("store: delete account: %w", err)
	}

	return nil
}

func (s *Postgres) LinkProMembership(ctx context.Context, accountID int64, email string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE accounts SET pro_email = $2 WHERE id = $1`, accountID, email)
	if isUnique(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: link pro membership: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) ProByEmail(ctx context.Context, email string) (ProMember, bool, error) {
	var m ProMember
	err := s.pool.QueryRow(ctx, `SELECT email, token, until FROM pro_members WHERE email = $1`, email).Scan(&m.Email, &m.Token, &m.Until)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProMember{}, false, nil
	}
	if err != nil {
		return ProMember{}, false, fmt.Errorf("store: pro by email: %w", err)
	}

	return m, true, nil
}

func (s *Postgres) ProByPayment(ctx context.Context, paymentID string) (ProMember, bool, error) {
	var m ProMember
	err := s.pool.QueryRow(ctx, `
		SELECT m.email, m.token, m.until FROM pro_payments p JOIN pro_members m ON m.email = p.email WHERE p.payment_id = $1`, paymentID).Scan(&m.Email, &m.Token, &m.Until)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProMember{}, false, nil
	}
	if err != nil {
		return ProMember{}, false, fmt.Errorf("store: pro by payment: %w", err)
	}

	return m, true, nil
}

func (s *Postgres) AccountByIdentity(ctx context.Context, provider, subject string) (Account, bool, error) {
	return s.accountWhere(ctx, "JOIN account_identities i ON i.account_id = a.id", "i.provider = $1 AND i.subject = $2", provider, subject)
}

func (s *Postgres) LinkIdentity(ctx context.Context, accountID int64, provider, subject string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO account_identities (provider, subject, account_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, provider, subject, accountID)
	if err != nil {
		return false, fmt.Errorf("store: link identity: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return true, nil
	}
	// Already linked: fine when it is to this account, refused when it is to another.
	var owner int64
	if err := s.pool.QueryRow(ctx, `SELECT account_id FROM account_identities WHERE provider = $1 AND subject = $2`, provider, subject).Scan(&owner); err != nil {
		return false, fmt.Errorf("store: link identity: %w", err)
	}

	return owner == accountID, nil
}

func (s *Postgres) Identities(ctx context.Context, accountID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT provider FROM account_identities WHERE account_id = $1 ORDER BY provider`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: identities: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("store: identities scan: %w", err)
		}
		out = append(out, p)
	}

	return out, rows.Err()
}

func (s *Postgres) AccountSync(ctx context.Context, accountID int64) (SyncData, error) {
	d := SyncData{Bookmarks: "[]", Cards: "{}", Missed: "[]"}
	err := s.pool.QueryRow(ctx, `SELECT bookmarks::text, cards::text, streak, missed::text FROM account_sync WHERE account_id = $1`, accountID).
		Scan(&d.Bookmarks, &d.Cards, &d.Streak, &d.Missed)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, nil
	}
	if err != nil {
		return SyncData{}, fmt.Errorf("store: account sync: %w", err)
	}

	return d, nil
}

func (s *Postgres) SetAccountSync(ctx context.Context, accountID int64, d SyncData) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO account_sync (account_id, bookmarks, cards, streak, missed) VALUES ($1, $2::jsonb, $3::jsonb, $4, $5::jsonb)
		ON CONFLICT (account_id) DO UPDATE SET bookmarks = $2::jsonb, cards = $3::jsonb, streak = $4, missed = $5::jsonb, updated_at = now()`,
		accountID, d.Bookmarks, d.Cards, d.Streak, d.Missed)
	if err != nil {
		return fmt.Errorf("store: set account sync: %w", err)
	}

	return nil
}
