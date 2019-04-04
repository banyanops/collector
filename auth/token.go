package auth

import "sync"

type TokenSyncInfo struct {
	sync.RWMutex
	Token       string
	Application string
}

func (ts *TokenSyncInfo) UpdateToken(token string) {
	ts.Lock()
	ts.Token = token
	ts.Unlock()
}

func (ts *TokenSyncInfo) GetToken() (token string) {
	ts.RLock()
	token = ts.Token
	ts.RUnlock()
	return
}

func (ts *TokenSyncInfo) SetApplication(application string) {
	ts.Application = application
}

func (ts *TokenSyncInfo) UpdateTokenLocked(token string) {
	ts.Token = token
}

func (ts *TokenSyncInfo) GetTokenLocked() (token string) {
	token = ts.Token
	return
}
