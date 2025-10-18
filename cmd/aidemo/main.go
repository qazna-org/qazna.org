package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"qazna.org/internal/ai/sim"
)

func main() {
	var (
		baseURL     = flag.String("base-url", "http://localhost:8080", "API base URL")
		workers     = flag.Int("workers", 4, "Concurrent worker count")
		duration    = flag.Duration("duration", 2*time.Minute, "Duration of the simulation")
		openAIModel = flag.String("openai-model", "gpt-4o-mini", "OpenAI model for summaries (optional)")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Printf("Launching AI demo: base=%s workers=%d duration=%s", *baseURL, *workers, *duration)

	token, err := issueToken(ctx, *baseURL)
	if err != nil {
		log.Fatalf("issue token: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	generator := sim.NewGenerator(time.Now().UnixNano())
	if err := ensureLedgerAccounts(ctx, client, *baseURL, token, &generator); err != nil {
		log.Fatalf("bootstrap ledger accounts: %v", err)
	}

	var counter sim.Counter
	var successes int64
	var failures int64
	var conflicts int64
	var rateLimited int64
	var serverErrors int64

	var wg sync.WaitGroup
	deadline := time.Now().Add(*duration)

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id*9973)))
			for time.Now().Before(deadline) {
				select {
				case <-ctx.Done():
					return
				default:
				}
				transfer := generator.NextTransfer()
				idem := uuid.NewString()
				payload := map[string]any{
					"from_id":         transfer.FromID,
					"to_id":           transfer.ToID,
					"currency":        transfer.Currency,
					"amount":          transfer.Amount,
					"idempotency_key": idem,
				}
				body, _ := json.Marshal(payload)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/transfers", *baseURL), bytes.NewReader(body))
				if err != nil {
					log.Printf("worker %d request: %v", id, err)
					atomic.AddInt64(&failures, 1)
					continue
				}
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", idem)
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("worker %d do: %v", id, err)
					atomic.AddInt64(&failures, 1)
					continue
				}
				resp.Body.Close()
				if resp.StatusCode >= 300 {
					atomic.AddInt64(&failures, 1)
					switch resp.StatusCode {
					case http.StatusConflict:
						atomic.AddInt64(&conflicts, 1)
					case http.StatusTooManyRequests:
						atomic.AddInt64(&rateLimited, 1)
						time.Sleep(250 * time.Millisecond)
					default:
						atomic.AddInt64(&serverErrors, 1)
						log.Printf("worker %d transfer failed: %s", id, resp.Status)
						time.Sleep(200 * time.Millisecond)
					}
					continue
				}
				atomic.AddInt64(&successes, 1)
				counter.Add(transfer)
				time.Sleep(time.Duration(50+rnd.Intn(120)) * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	log.Printf("Run complete: %d success / %d failed (conflicts=%d, rate_limited=%d, server_errors=%d), volume %.2f %s", successes, failures, conflicts, rateLimited, serverErrors, counter.MajorAmount(), counter.Currency)

	if key := os.Getenv("OPENAI_API_KEY"); key != "" && counter.Transfers > 0 {
		summary, err := sim.Summarize(ctx, sim.SummaryStats{
			Transfers:   counter.Transfers,
			TotalAmount: counter.MajorAmount(),
			Currency:    counter.Currency,
			Duration:    *duration,
		}, sim.SummaryRequest{APIKey: key, Model: *openAIModel})
		if err != nil {
			log.Printf("AI summary error: %v", err)
		} else {
			log.Println("AI Executive Summary:")
			log.Println(summary)
		}
	} else {
		log.Println("Set OPENAI_API_KEY to enable AI narrative summaries.")
	}
}

func issueToken(ctx context.Context, baseURL string) (string, error) {
	payload := map[string]any{
		"user":  "demo-ai",
		"roles": []string{"admin"},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/auth/token", baseURL), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("token endpoint: %s", resp.Status)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", errors.New("empty token returned")
	}
	return out.Token, nil
}

func ensureLedgerAccounts(ctx context.Context, client *http.Client, baseURL, token string, generator *sim.Generator) error {
	desired := generator.Accounts()
	actual := make([]sim.Account, 0, len(desired))
	for _, acc := range desired {
		created, err := createLedgerAccount(ctx, client, baseURL, token, acc)
		if err != nil {
			return fmt.Errorf("create account %s: %w", acc.Label, err)
		}
		actual = append(actual, created)
		log.Printf("Provisioned ledger account %s (%s) with balance %.2f %s", created.Label, created.ID, float64(created.Initial)/100, created.Currency)
	}
	generator.OverrideAccounts(actual)
	return nil
}

func createLedgerAccount(ctx context.Context, client *http.Client, baseURL, token string, spec sim.Account) (sim.Account, error) {
	if spec.Initial < 0 {
		spec.Initial = 0
	}
	body, _ := json.Marshal(map[string]any{
		"currency":       spec.Currency,
		"initial_amount": spec.Initial,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/accounts", baseURL), bytes.NewReader(body))
	if err != nil {
		return sim.Account{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return sim.Account{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return sim.Account{}, fmt.Errorf("status %s: %s", resp.Status, string(bytes.TrimSpace(b)))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return sim.Account{}, err
	}
	if out.ID == "" {
		return sim.Account{}, errors.New("empty id returned")
	}
	return sim.Account{
		ID:       out.ID,
		Currency: spec.Currency,
		Label:    spec.Label,
		Initial:  spec.Initial,
	}, nil
}
