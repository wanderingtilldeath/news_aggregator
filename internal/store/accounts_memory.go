package store

import (
	"context"
	"sort"
	"strings"
	"time"
)

// memAccount is an account and its recovery codes (hash -> spent).
type memAccount struct {
	Account
	codes map[string]bool
}

type memSession struct {
	accountID int64
	expires   time.Time
}

func (m *Memory) initAccounts() {
	if m.accounts == nil {
		m.accounts = map[string]*memAccount{}
		m.acctSession = map[string]memSession{}
		m.identities = map[string]int64{}
	}
}

func (m *Memory) byID(id int64) *memAccount {
	for _, a := range m.accounts {
		if a.ID == id {
			return a
		}
	}

	return nil
}

func (m *Memory) CreateAccount(_ context.Context, username, passwordHash string, codeHashes []string) (Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initAccounts()
	if _, taken := m.accounts[username]; taken {
		return Account{}, false, nil
	}
	m.accountSeq++
	a := &memAccount{Account: Account{ID: m.accountSeq, Username: username, PasswordHash: passwordHash}, codes: map[string]bool{}}
	for _, h := range codeHashes {
		a.codes[h] = false
	}
	m.accounts[username] = a

	return a.Account, true, nil
}

func (m *Memory) AccountByUsername(_ context.Context, username string) (Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if a, ok := m.accounts[username]; ok {
		return a.Account, true, nil
	}

	return Account{}, false, nil
}

func (m *Memory) AccountByEmail(_ context.Context, email string) (Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, a := range m.accounts {
		if a.Email == email && a.EmailVerified {
			return a.Account, true, nil
		}
	}

	return Account{}, false, nil
}

func (m *Memory) SetAccountEmail(_ context.Context, id int64, email string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if a := m.byID(id); a != nil {
		a.Email, a.EmailVerified = email, false
	}
	for h, t := range m.emailTokens {
		if t.accountID == id {
			delete(m.emailTokens, h)
		}
	}

	return true, nil
}

func (m *Memory) VerifyAccountEmail(_ context.Context, id int64, email string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a := m.byID(id)
	if a == nil || a.Email != email {
		return false, nil
	}
	for _, other := range m.accounts {
		if other.ID != id && other.Email == email && other.EmailVerified {
			return false, nil
		}
	}
	a.EmailVerified = true

	return true, nil
}

type memEmailToken struct {
	accountID int64
	purpose   string
	email     string
	expires   time.Time
}

func (m *Memory) SaveEmailToken(_ context.Context, hash string, accountID int64, purpose, email string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.emailTokens == nil {
		m.emailTokens = map[string]memEmailToken{}
	}
	for h, t := range m.emailTokens {
		if t.accountID == accountID && t.purpose == purpose {
			delete(m.emailTokens, h)
		}
	}
	m.emailTokens[hash] = memEmailToken{accountID, purpose, email, expires}

	return nil
}

func (m *Memory) UseEmailToken(_ context.Context, hash, purpose string, now time.Time) (Account, string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.emailTokens[hash]
	if !ok || t.purpose != purpose || !t.expires.After(now) {
		return Account{}, "", false, nil
	}
	delete(m.emailTokens, hash)
	a := m.byID(t.accountID)
	if a == nil || a.Email != t.email {
		return Account{}, "", false, nil
	}

	return a.Account, t.email, true, nil
}

func (m *Memory) SetAccountPassword(_ context.Context, id int64, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if a := m.byID(id); a != nil {
		a.PasswordHash = passwordHash
	}

	return nil
}

func (m *Memory) ReplaceRecoveryCodes(_ context.Context, id int64, codeHashes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if a := m.byID(id); a != nil {
		a.codes = map[string]bool{}
		for _, h := range codeHashes {
			a.codes[h] = false
		}
	}

	return nil
}

func (m *Memory) UseRecoveryCode(_ context.Context, id int64, codeHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a := m.byID(id)
	if a == nil {
		return false, nil
	}
	if spent, ok := a.codes[codeHash]; !ok || spent {
		return false, nil
	}
	a.codes[codeHash] = true

	return true, nil
}

func (m *Memory) RecoveryCodesLeft(_ context.Context, id int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := 0
	if a := m.byID(id); a != nil {
		for _, spent := range a.codes {
			if !spent {
				n++
			}
		}
	}

	return n, nil
}

func (m *Memory) SaveAccountSession(_ context.Context, hash string, accountID int64, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initAccounts()
	now := time.Now()
	for h, s := range m.acctSession {
		if s.expires.Before(now) {
			delete(m.acctSession, h)
		}
	}
	m.acctSession[hash] = memSession{accountID: accountID, expires: expires}

	return nil
}

func (m *Memory) AccountBySession(_ context.Context, hash string, now time.Time) (Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.acctSession[hash]
	if !ok || !s.expires.After(now) {
		return Account{}, false, nil
	}
	if a := m.byID(s.accountID); a != nil {
		return a.Account, true, nil
	}

	return Account{}, false, nil
}

func (m *Memory) DeleteAccountSession(_ context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.acctSession, hash)

	return nil
}

func (m *Memory) DeleteAccountSessions(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for h, s := range m.acctSession {
		if s.accountID == id {
			delete(m.acctSession, h)
		}
	}

	return nil
}

func (m *Memory) DeleteAccount(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if a := m.byID(id); a != nil {
		delete(m.accounts, a.Username)
	}
	delete(m.sync, id)
	for h, t := range m.emailTokens {
		if t.accountID == id {
			delete(m.emailTokens, h)
		}
	}
	for h, s := range m.acctSession {
		if s.accountID == id {
			delete(m.acctSession, h)
		}
	}
	for k, owner := range m.identities {
		if owner == id {
			delete(m.identities, k)
		}
	}

	return nil
}

func (m *Memory) LinkProMembership(_ context.Context, accountID int64, email string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, a := range m.accounts {
		if a.ID != accountID && a.ProEmail == email {
			return false, nil
		}
	}
	a := m.byID(accountID)
	if a == nil {
		return false, nil
	}
	a.ProEmail = email

	return true, nil
}

func (m *Memory) ProByEmail(_ context.Context, email string) (ProMember, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	member, ok := m.pro[email]

	return member, ok, nil
}

func (m *Memory) ProByPayment(_ context.Context, paymentID string) (ProMember, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	email, ok := m.payments[paymentID]
	if !ok {
		return ProMember{}, false, nil
	}
	member, ok := m.pro[email]

	return member, ok, nil
}

func (m *Memory) AccountByIdentity(_ context.Context, provider, subject string) (Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id, ok := m.identities[provider+"|"+subject]; ok {
		if a := m.byID(id); a != nil {
			return a.Account, true, nil
		}
	}

	return Account{}, false, nil
}

func (m *Memory) LinkIdentity(_ context.Context, accountID int64, provider, subject string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initAccounts()
	key := provider + "|" + subject
	if owner, ok := m.identities[key]; ok {
		return owner == accountID, nil
	}
	m.identities[key] = accountID

	return true, nil
}

func (m *Memory) Identities(_ context.Context, accountID int64) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := []string{}
	for key, owner := range m.identities {
		if owner == accountID {
			out = append(out, key[:strings.Index(key, "|")])
		}
	}
	sort.Strings(out)

	return out, nil
}

func (m *Memory) AccountSync(_ context.Context, accountID int64) (SyncData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if d, ok := m.sync[accountID]; ok {
		return d, nil
	}

	return SyncData{Bookmarks: "[]", Cards: "{}", Missed: "[]", Marks: "[]"}, nil
}

func (m *Memory) SetAccountSync(_ context.Context, accountID int64, d SyncData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sync == nil {
		m.sync = map[int64]SyncData{}
	}
	m.sync[accountID] = d

	return nil
}
