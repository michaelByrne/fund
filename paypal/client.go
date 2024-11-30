package paypal

import (
	"boardfund/paypal/token"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	paypalAuth *token.Store
	httpClient *http.Client
	baseURL    string
}

func NewClient(paypalAuth *token.Store, baseURL string) *Client {
	return &Client{
		paypalAuth: paypalAuth,
		httpClient: http.DefaultClient,
		baseURL:    baseURL,
	}
}

func (c Client) post(ctx context.Context, path string, payload []byte) error {
	token, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return err
	}

	payloadReader := bytes.NewReader(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, payloadReader)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", token.TokenType+" "+token.AccessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return nil
}

func (c Client) postWithResponse(ctx context.Context, path string, payload []byte) ([]byte, error) {
	paypalToken, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	payloadReader := bytes.NewReader(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, payloadReader)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", paypalToken.TokenType+" "+paypalToken.AccessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		var paypalError ErrPaypal
		err = json.Unmarshal(body, &paypalError)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling error response: %w", err)
		}

		fmt.Printf("paypal error: %v\n", paypalError.Details)

		return nil, paypalError
	}

	return body, nil
}

func (c Client) get(ctx context.Context, path string) ([]byte, error) {
	token, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", token.TokenType+" "+token.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (c Client) patch(ctx context.Context, path string, payload []byte) error {
	token, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return err
	}

	payloadReader := bytes.NewReader(payload)

	req, err := http.NewRequestWithContext(ctx, "PATCH", c.baseURL+path, payloadReader)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", token.TokenType+" "+token.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return nil
}

func (c Client) delete(ctx context.Context, path string) error {
	token, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+path, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", token.TokenType+" "+token.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return nil
}
