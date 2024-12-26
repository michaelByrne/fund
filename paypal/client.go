package paypal

import (
	"boardfund/paypal/token"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type Client struct {
	paypalAuth *token.Store
	httpClient *http.Client
	logger     *slog.Logger
	baseURL    string
}

func NewClient(paypalAuth *token.Store, logger *slog.Logger, baseURL string) *Client {
	return &Client{
		paypalAuth: paypalAuth,
		httpClient: http.DefaultClient,
		logger:     logger,
		baseURL:    baseURL,
	}
}

func (c Client) post(ctx context.Context, path string, payload any) error {
	token, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return err
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	payloadReader := bytes.NewReader(payloadBytes)

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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var paypalErr ErrPaypal
		err = json.NewDecoder(resp.Body).Decode(&paypalErr)
		if err != nil {
			return fmt.Errorf("error decoding paypal error: %w", err)
		}

		c.logger.Error("error from paypal", slog.Any("details", paypalErr.Details), slog.String("message", paypalErr.Message))

		return paypalErr
	}

	defer resp.Body.Close()

	return nil
}

func (c Client) postWithResponse(ctx context.Context, path string, payload any) ([]byte, error) {
	paypalToken, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	payloadReader := bytes.NewReader(payloadBytes)

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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var paypalErr ErrPaypal
		err = json.Unmarshal(body, &paypalErr)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling error response: %w", err)
		}

		c.logger.Error("error from paypal", slog.Any("details", paypalErr.Details), slog.String("message", paypalErr.Message))

		return nil, paypalErr
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var paypalErr ErrPaypal
		err = json.Unmarshal(body, &paypalErr)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling error response: %w", err)
		}

		c.logger.Error("error from paypal", slog.Any("details", paypalErr.Details), slog.String("message", paypalErr.Message))

		return nil, paypalErr
	}

	return body, nil
}

func (c Client) patch(ctx context.Context, path string, payload any) error {
	token, err := c.paypalAuth.GetToken(ctx)
	if err != nil {
		return err
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	fmt.Printf("payloadBytes: %+v\n", string(payloadBytes))

	payloadReader := bytes.NewReader(payloadBytes)

	req, err := http.NewRequestWithContext(ctx, "PATCH", c.baseURL+path, payloadReader)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {
		var paypalErr ErrPaypal
		err = json.Unmarshal(body, &paypalErr)
		if err != nil {
			return fmt.Errorf("error unmarshalling error response: %w", err)
		}

		c.logger.Error("error from paypal", slog.Any("details", paypalErr.Details), slog.String("message", paypalErr.Message))

		return paypalErr
	}

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

	if resp.StatusCode != http.StatusCreated {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var paypalErr ErrPaypal
		err = json.Unmarshal(body, &paypalErr)
		if err != nil {
			return fmt.Errorf("error unmarshalling error response: %w", err)
		}

		c.logger.Error("error from paypal", slog.Any("details", paypalErr.Details), slog.String("message", paypalErr.Message))

		return paypalErr
	}

	return nil
}
