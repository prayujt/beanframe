// Package rpcserver exposes the shared ledger service through ConnectRPC.
package rpcserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	pb "github.com/prayujt/beanframe/apps/server/gen/beancount/v1"
	"github.com/prayujt/beanframe/apps/server/gen/beancount/v1/beancountv1connect"
	"github.com/prayujt/beanframe/apps/server/internal/auth"
	"github.com/prayujt/beanframe/apps/server/internal/config"
	"github.com/prayujt/beanframe/apps/server/internal/engine"
	"github.com/prayujt/beanframe/apps/server/internal/ledger"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	service  *ledger.Service
	branding config.Branding
}

func Handler(s *ledger.Service, branding config.Branding) (string, http.Handler) {
	return beancountv1connect.NewLedgerServiceHandler(&Server{service: s, branding: branding}, connect.WithReadMaxBytes(3<<20), connect.WithSendMaxBytes(32<<20))
}
func problem(err error) error {
	var e *engine.Error
	if errors.As(err, &e) {
		codes := map[string]connect.Code{"aborted": connect.CodeAborted, "invalid_argument": connect.CodeInvalidArgument, "not_found": connect.CodeNotFound, "already_exists": connect.CodeAlreadyExists, "failed_precondition": connect.CodeFailedPrecondition}
		code, ok := codes[e.Code]
		if !ok {
			code = connect.CodeInternal
		}
		return connect.NewError(code, errors.New(e.Message))
	}
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, err)
	}
	return connect.NewError(connect.CodeInternal, errors.New("The ledger could not be loaded. Check the server logs and file permissions."))
}
func (s *Server) query(ctx context.Context, op string, in proto.Message, out proto.Message) error {
	var args any
	if in != nil {
		raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(in)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &args); err != nil {
			return err
		}
	}
	raw, err := s.service.Query(ctx, op, args)
	if err != nil {
		return problem(err)
	}
	if err = protojson.Unmarshal(raw, out); err != nil {
		return problem(err)
	}
	return nil
}
func (s *Server) GetSession(ctx context.Context, _ *connect.Request[pb.Empty]) (*connect.Response[pb.Session], error) {
	a := auth.FromContext(ctx)
	v := &pb.Session{Authenticated: a.Authenticated, AuthEnabled: a.AuthEnabled, Provider: a.Provider, CanWrite: a.CanWrite, CompanyName: s.branding.CompanyName, BrandName: s.branding.Name, BrandLogoUrl: s.branding.LogoURL, Renewable: a.Renewable}
	if a.Authenticated {
		v.User = &pb.User{Subject: a.User.Subject, Name: a.User.Name, Username: a.User.Username, Email: a.User.Email, Groups: a.User.Groups}
		v.ExpiresAt = a.Expires.Format(time.RFC3339)
	}
	return connect.NewResponse(v), nil
}
func (s *Server) GetSnapshot(ctx context.Context, _ *connect.Request[pb.Empty]) (*connect.Response[pb.Snapshot], error) {
	v := new(pb.Snapshot)
	if err := s.query(ctx, "snapshot", nil, v); err != nil {
		return nil, err
	}
	return connect.NewResponse(v), nil
}
func (s *Server) GetReport(ctx context.Context, r *connect.Request[pb.ReportRequest]) (*connect.Response[pb.Report], error) {
	v := new(pb.Report)
	if err := s.query(ctx, "report", r.Msg, v); err != nil {
		return nil, err
	}
	return connect.NewResponse(v), nil
}
func (s *Server) ReadFile(ctx context.Context, r *connect.Request[pb.FileRequest]) (*connect.Response[pb.LedgerFile], error) {
	v := new(pb.LedgerFile)
	if err := s.query(ctx, "read_file", r.Msg, v); err != nil {
		return nil, err
	}
	return connect.NewResponse(v), nil
}
func (s *Server) GetHistory(ctx context.Context, _ *connect.Request[pb.Empty]) (*connect.Response[pb.History], error) {
	v := new(pb.History)
	if err := s.query(ctx, "history", nil, v); err != nil {
		return nil, err
	}
	return connect.NewResponse(v), nil
}
func (s *Server) mutate(ctx context.Context, op string, in proto.Message) (*connect.Response[pb.MutationResult], error) {
	a := auth.FromContext(ctx)
	if !a.CanWrite || !a.Valid() {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("Write access is required"))
	}
	raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(in)
	if err != nil {
		return nil, err
	}
	var args ledger.Mutation
	if err = json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	args.Actor = a.Actor
	result, err := s.service.Mutate(ctx, op, args)
	if err != nil {
		return nil, problem(err)
	}
	return connect.NewResponse(&pb.MutationResult{Revision: result.Revision, Message: result.Message}), nil
}
func (s *Server) SaveTransaction(ctx context.Context, r *connect.Request[pb.SaveTransactionRequest]) (*connect.Response[pb.MutationResult], error) {
	return s.mutate(ctx, "save_transaction", r.Msg)
}
func (s *Server) DeleteTransaction(ctx context.Context, r *connect.Request[pb.DeleteTransactionRequest]) (*connect.Response[pb.MutationResult], error) {
	return s.mutate(ctx, "delete_transaction", r.Msg)
}
func (s *Server) OpenAccount(ctx context.Context, r *connect.Request[pb.OpenAccountRequest]) (*connect.Response[pb.MutationResult], error) {
	return s.mutate(ctx, "open_account", r.Msg)
}
func (s *Server) CloseAccount(ctx context.Context, r *connect.Request[pb.CloseAccountRequest]) (*connect.Response[pb.MutationResult], error) {
	return s.mutate(ctx, "close_account", r.Msg)
}
func (s *Server) WriteFile(ctx context.Context, r *connect.Request[pb.WriteFileRequest]) (*connect.Response[pb.MutationResult], error) {
	return s.mutate(ctx, "write_file", r.Msg)
}
func (s *Server) RestoreVersion(ctx context.Context, r *connect.Request[pb.RestoreVersionRequest]) (*connect.Response[pb.MutationResult], error) {
	return s.mutate(ctx, "restore_version", r.Msg)
}
func (s *Server) WatchLedger(ctx context.Context, _ *connect.Request[pb.Empty], stream *connect.ServerStream[pb.LedgerEvent]) error {
	ch, unsubscribe := s.service.Subscribe()
	defer unsubscribe()
	raw, err := s.service.Query(ctx, "revision", nil)
	if err != nil {
		return problem(err)
	}
	var initial ledger.Event
	if err = json.Unmarshal(raw, &initial); err != nil {
		return problem(err)
	}
	if err = stream.Send(&pb.LedgerEvent{Revision: initial.Revision, Kind: "connected"}); err != nil {
		return err
	}
	heartbeat := time.NewTicker(time.Second)
	defer heartbeat.Stop()
	lastHeartbeat := time.Now()
	for {
		if !auth.FromContext(ctx).Valid() {
			return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("Session expired"))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-ch:
			if err := stream.Send(&pb.LedgerEvent{Revision: event.Revision, Kind: event.Kind}); err != nil {
				return err
			}
		case <-heartbeat.C:
			if time.Since(lastHeartbeat) >= 15*time.Second {
				if err := stream.Send(&pb.LedgerEvent{Kind: "heartbeat"}); err != nil {
					return err
				}
				lastHeartbeat = time.Now()
			}
		}
	}
}
