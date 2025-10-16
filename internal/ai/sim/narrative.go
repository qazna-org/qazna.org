package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type SummaryStats struct {
	Transfers   int
	TotalAmount float64
	Currency    string
	Duration    time.Duration
}

type SummaryRequest struct {
	Model  string
	APIKey string
}

func Summarize(ctx context.Context, stats SummaryStats, req SummaryRequest) (string, error) {
	if req.APIKey == "" {
		return "", errors.New("missing API key")
	}
	if req.Model == "" {
		req.Model = "gpt-4o-mini"
	}
	payload := map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a central bank operations analyst summarizing live settlement telemetry."},
			{"role": "user", "content": fmt.Sprintf("Transfers: %d, Volume: %.2f %s, Window: %s. Provide a concise executive summary (max 3 sentences).", stats.Transfers, stats.TotalAmount, stats.Currency, stats.Duration)},
		},
		"temperature": 0.2,
	}
	buf, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai error: %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("no choices returned")
	}
	return out.Choices[0].Message.Content, nil
}
