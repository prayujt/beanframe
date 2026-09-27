package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Event struct {
	Revision string `json:"revision"`
	Kind     string `json:"kind"`
}
type Mutation struct {
	ExpectedRevision string   `json:"expected_revision"`
	ID               string   `json:"id,omitempty"`
	File             string   `json:"file,omitempty"`
	Source           string   `json:"source,omitempty"`
	Name             string   `json:"name,omitempty"`
	Date             string   `json:"date,omitempty"`
	Currencies       []string `json:"currencies,omitempty"`
	Path             string   `json:"path,omitempty"`
	Content          string   `json:"content,omitempty"`
	Actor            string   `json:"actor,omitempty"`
}
type MutationResult struct {
	Revision string `json:"revision"`
	Message  string `json:"message"`
}

func (s *Service) Query(ctx context.Context, op string, args any) (json.RawMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Workspace == nil {
		return nil, fmt.Errorf("workspace is unavailable")
	}
	return s.Workspace.Call(ctx, op, args)
}
func (s *Service) Mutate(ctx context.Context, op string, args Mutation) (*MutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Workspace == nil {
		return nil, fmt.Errorf("workspace is unavailable")
	}
	raw, err := s.Workspace.Call(ctx, op, args)
	if err != nil {
		return nil, err
	}
	var result MutationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	s.Publish(Event{Revision: result.Revision, Kind: "saved"})
	return &result, nil
}
func (s *Service) Publish(event Event) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if event.Revision == s.revision {
		return
	}
	s.revision = event.Revision
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- event
		}
	}
}
func (s *Service) Subscribe() (<-chan Event, func()) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.subscribers == nil {
		s.subscribers = make(map[chan Event]struct{})
	}
	ch := make(chan Event, 1)
	s.subscribers[ch] = struct{}{}
	return ch, func() { s.eventMu.Lock(); delete(s.subscribers, ch); s.eventMu.Unlock() }
}
func (s *Service) Watch(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		raw, err := s.Query(ctx, "revision", nil)
		if err != nil {
			continue
		}
		var event Event
		if json.Unmarshal(raw, &event) == nil {
			event.Kind = "changed"
			s.Publish(event)
		}
	}
}
