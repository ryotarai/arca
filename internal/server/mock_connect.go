package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
	"github.com/ryotarai/arca/internal/machine"
)

type mockConnectService struct {
	runtime       *machine.MockRuntime
	authenticator Authenticator
	store         *db.Store
}

func newMockConnectService(runtime *machine.MockRuntime, authenticator Authenticator, store *db.Store) *mockConnectService {
	return &mockConnectService{
		runtime:       runtime,
		authenticator: authenticator,
		store:         store,
	}
}

func (s *mockConnectService) SetDefaultBehavior(ctx context.Context, req *connect.Request[arcav1.SetDefaultBehaviorRequest]) (*connect.Response[arcav1.SetDefaultBehaviorResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	b := convertBehavior(req.Msg.GetBehavior())
	s.runtime.SetDefaultBehavior(b)

	return connect.NewResponse(&arcav1.SetDefaultBehaviorResponse{}), nil
}

func (s *mockConnectService) SetMachineBehavior(ctx context.Context, req *connect.Request[arcav1.SetMachineBehaviorRequest]) (*connect.Response[arcav1.SetMachineBehaviorResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	machineID := req.Msg.GetMachineId()
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine_id is required"))
	}

	b := convertBehavior(req.Msg.GetBehavior())
	s.runtime.SetMachineBehavior(machineID, b)

	return connect.NewResponse(&arcav1.SetMachineBehaviorResponse{}), nil
}

func (s *mockConnectService) ResetBehavior(ctx context.Context, req *connect.Request[arcav1.ResetBehaviorRequest]) (*connect.Response[arcav1.ResetBehaviorResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	s.runtime.ResetBehavior()

	return connect.NewResponse(&arcav1.ResetBehaviorResponse{}), nil
}

func (s *mockConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("mock service unavailable"))
	}
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return "", err
	}
	if result.Role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage mock behavior"))
	}
	return result.UserID, nil
}

func convertBehavior(pb *arcav1.MockBehavior) machine.MockBehavior {
	if pb == nil {
		return machine.MockBehavior{}
	}
	return machine.MockBehavior{
		Delay:   time.Duration(pb.GetDelayMs()) * time.Millisecond,
		ErrorOn: pb.GetErrorOn(),
	}
}
