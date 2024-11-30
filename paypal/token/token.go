package token

import (
	"boardfund/cache"
	"context"
	"time"
)

const tokenKey = "paypal_token"

type Token struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
	AppID       string `json:"app_id"`
	ExpiresIn   int    `json:"expires_in"`
	Nonce       string `json:"nonce"`
}

type Store struct {
	cache  *cache.TTLCache[string, Token]
	client Client
}

func NewStore(client Client) *Store {
	return &Store{
		cache:  cache.NewTTLCache[string, Token](),
		client: client,
	}
}

func (p *Store) GetToken(ctx context.Context) (*Token, error) {
	if token, ok := p.cache.Get(tokenKey); ok {
		return &token, nil
	}

	token, err := p.client.fetchToken(ctx)
	if err != nil {
		return nil, err
	}

	p.cache.Set(tokenKey, token, time.Duration(token.ExpiresIn)*time.Second)

	return &token, nil
}
