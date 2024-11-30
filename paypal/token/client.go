package token

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client
}

func NewClient(clientID, clientSecret, baseURL string) Client {
	return Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      baseURL,
		httpClient:   http.DefaultClient,
	}
}

func (c *Client) fetchToken(ctx context.Context) (Token, error) {
	values := url.Values{}
	values.Add("grant_type", "client_credentials")
	bodyStr := values.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/oauth2/token", strings.NewReader(bodyStr))
	if err != nil {
		return Token{}, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Token{}, err
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return Token{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return Token{}, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, bodyBytes)
	}

	var token Token
	err = json.Unmarshal(bodyBytes, &token)
	if err != nil {
		return Token{}, err
	}

	return token, nil
}
