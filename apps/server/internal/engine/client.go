// Package engine connects Go to the bundled Python Beancount library.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type Amount struct {
	Number   string `json:"number"`
	Currency string `json:"currency"`
}
type Position struct {
	Units *Amount `json:"units"`
	Cost  *string `json:"cost"`
}
type Posting struct {
	BookValue *Amount `json:"book_value"`
	Account   string  `json:"account"`
	Units     *Amount `json:"units"`
	Cost      *string `json:"cost"`
	Price     *Amount `json:"price"`
}
type Transaction struct {
	ID        string    `json:"id"`
	File      string    `json:"file"`
	Line      int       `json:"line"`
	Source    string    `json:"source"`
	Date      string    `json:"date"`
	Flag      string    `json:"flag"`
	Payee     string    `json:"payee"`
	Narration string    `json:"narration"`
	Tags      []string  `json:"tags"`
	Links     []string  `json:"links"`
	Postings  []Posting `json:"postings"`
}
type MutationEvent struct {
	Type        string       `json:"type"`
	File        string       `json:"file,omitempty"`
	Account     string       `json:"account,omitempty"`
	Date        string       `json:"date,omitempty"`
	Transaction *Transaction `json:"transaction,omitempty"`
}
type Account struct {
	Closed     string   `json:"closed"`
	Name       string   `json:"name"`
	Opened     string   `json:"opened"`
	Currencies []string `json:"currencies"`
}
type Balance struct {
	Account   string     `json:"account"`
	Positions []Position `json:"positions"`
}
type Snapshot struct {
	Revision            string        `json:"revision"`
	Diagnostics         []Diagnostic  `json:"diagnostics"`
	Files               []FileInfo    `json:"files"`
	Currencies          []string      `json:"currencies"`
	OperatingCurrencies []string      `json:"operating_currencies"`
	Title               string        `json:"title"`
	Accounts            []Account     `json:"accounts"`
	Transactions        []Transaction `json:"transactions"`
	Balances            []Balance     `json:"balances"`
	Errors              []string      `json:"errors"`
}

type Diagnostic struct {
	Message string `json:"message"`
	File    string `json:"file"`
	Line    int    `json:"line"`
}
type FileInfo struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"error"`
}

func (e *Error) Error() string { return e.Message }

type Client struct {
	Python string
	Path   string
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("engine output limit exceeded")
	}
	return b.Buffer.Write(p)
}

// Snapshot reloads the ledger for every operation so external edits are visible.
func (c *Client) Snapshot(ctx context.Context) (*Snapshot, error) {
	raw, err := c.Call(ctx, "snapshot", nil)
	if err != nil {
		return nil, err
	}
	var result Snapshot
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Call(ctx context.Context, operation string, args any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := json.Marshal(map[string]any{"operation": operation, "path": c.Path, "args": args})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, c.Python, "-m", "beancount_engine")
	cmd.Stdin = bytes.NewReader(request)
	out := &boundedBuffer{limit: 32 << 20}
	diagnostic := &boundedBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, diagnostic
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Beancount engine failed: %w: %s", err, diagnostic.String())
	}
	var problem Error
	if err := json.Unmarshal(out.Bytes(), &problem); err != nil {
		return nil, fmt.Errorf("decode engine response: %w", err)
	}
	if problem.Message != "" {
		return nil, &problem
	}
	return json.RawMessage(out.Bytes()), nil
}
