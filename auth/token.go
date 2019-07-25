package auth

import "sync"

type TokenSyncInfo struct {
	sync.RWMutex
	Token        string
	Application  string
	RefreshToken string
}

func (ts *TokenSyncInfo) UpdateToken(token string) {
	ts.Lock()
	defer ts.Unlock()

	ts.Token = token
}

func (ts *TokenSyncInfo) GetToken() (token string) {
	ts.RLock()
	defer ts.RUnlock()

	return ts.Token
}

func (ts *TokenSyncInfo) GetRefreshToken() (token string) {
	ts.RLock()
	defer ts.RUnlock()

	return ts.RefreshToken
}

func (ts *TokenSyncInfo) SetApplication(application string) {
	ts.Application = application
}

func (ts *TokenSyncInfo) UpdateTokenLocked(token string) {
	ts.Token = token
}

func (ts *TokenSyncInfo) GetTokenLocked() (token string) {
	return ts.Token
}
