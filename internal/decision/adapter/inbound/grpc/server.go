package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/application"
	"github.com/mushroomyuan/vpp-backend/decision/application/command"
	"github.com/mushroomyuan/vpp-backend/decision/application/query"
)

// Server implements DecisionServiceServer.
type Server struct {
	decisionpb.UnimplementedDecisionServiceServer

	create  command.CreatePolicyHandler
	update  command.UpdatePolicyHandler
	delete  command.DeletePolicyHandler
	enable  command.EnablePolicyHandler
	disable command.DisablePolicyHandler
	get     query.GetPolicyHandler
	list    query.ListPoliciesHandler
}

// NewServer binds the policy API to the application handlers.
func NewServer(app application.Application) *Server {
	return &Server{
		create:  app.Commands.CreatePolicy,
		update:  app.Commands.UpdatePolicy,
		delete:  app.Commands.DeletePolicy,
		enable:  app.Commands.EnablePolicy,
		disable: app.Commands.DisablePolicy,
		get:     app.Queries.GetPolicy,
		list:    app.Queries.ListPolicies,
	}
}

func (s *Server) CreatePolicy(ctx context.Context, req *decisionpb.CreatePolicyRequest) (*decisionpb.Policy, error) {
	if req == nil {
		return nil, toGRPCError(errNilRequest())
	}
	kind, err := kindFromProto(req.GetKind())
	if err != nil {
		return nil, toGRPCError(err)
	}
	scope, err := scopeFromProto(req.GetScope())
	if err != nil {
		return nil, toGRPCError(err)
	}
	created, err := s.create.Handle(ctx, command.CreatePolicy{
		TenantID: req.GetTenantID(),
		Name:     req.GetName(),
		Kind:     kind,
		Scope:    scope,
		Cooldown: cooldownFromProto(req.GetCooldown()),
		SOC:      socFromProto(req.GetSOC()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return policyToProto(created), nil
}

func (s *Server) GetPolicy(ctx context.Context, req *decisionpb.GetPolicyRequest) (*decisionpb.Policy, error) {
	if req == nil {
		return nil, toGRPCError(errNilRequest())
	}
	found, err := s.get.Handle(ctx, query.GetPolicy{TenantID: req.GetTenantID(), ID: req.GetID()})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return policyToProto(found), nil
}

func (s *Server) ListPolicies(ctx context.Context, req *decisionpb.ListPoliciesRequest) (*decisionpb.ListPoliciesResponse, error) {
	if req == nil {
		return nil, toGRPCError(errNilRequest())
	}
	items, err := s.list.Handle(ctx, query.ListPolicies{
		TenantID:    req.GetTenantID(),
		EnabledOnly: req.GetEnabledOnly(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	out := &decisionpb.ListPoliciesResponse{Policies: make([]*decisionpb.Policy, 0, len(items))}
	for _, item := range items {
		out.Policies = append(out.Policies, policyToProto(item))
	}
	return out, nil
}

func (s *Server) UpdatePolicy(ctx context.Context, req *decisionpb.UpdatePolicyRequest) (*decisionpb.Policy, error) {
	if req == nil {
		return nil, toGRPCError(errNilRequest())
	}
	scope, err := scopeFromProto(req.GetScope())
	if err != nil {
		return nil, toGRPCError(err)
	}
	updated, err := s.update.Handle(ctx, command.UpdatePolicy{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
		Version:  req.GetVersion(),
		Name:     req.GetName(),
		Scope:    scope,
		Cooldown: cooldownFromProto(req.GetCooldown()),
		SOC:      socFromProto(req.GetSOC()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return policyToProto(updated), nil
}

func (s *Server) DeletePolicy(ctx context.Context, req *decisionpb.DeletePolicyRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, toGRPCError(errNilRequest())
	}
	if _, err := s.delete.Handle(ctx, command.DeletePolicy{TenantID: req.GetTenantID(), ID: req.GetID()}); err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) EnablePolicy(ctx context.Context, req *decisionpb.EnablePolicyRequest) (*decisionpb.Policy, error) {
	if req == nil {
		return nil, toGRPCError(errNilRequest())
	}
	enabled, err := s.enable.Handle(ctx, command.EnablePolicy{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
		Version:  req.GetVersion(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return policyToProto(enabled), nil
}

func (s *Server) DisablePolicy(ctx context.Context, req *decisionpb.DisablePolicyRequest) (*decisionpb.Policy, error) {
	if req == nil {
		return nil, toGRPCError(errNilRequest())
	}
	disabled, err := s.disable.Handle(ctx, command.DisablePolicy{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
		Version:  req.GetVersion(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return policyToProto(disabled), nil
}
