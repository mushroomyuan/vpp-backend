// Package application wires policy command and query handlers.
package application

import (
	"github.com/mushroomyuan/vpp-backend/decision/application/command"
	"github.com/mushroomyuan/vpp-backend/decision/application/query"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// Dependencies are the ports the policy API needs.
type Dependencies struct {
	Policies policy.Repository
	Resource port.ResourcePort
	Metrics  decorator.MetricsClient
}

// Application is the policy API. The decision cycle is wired in the process composition root.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands mutate policies.
type Commands struct {
	CreatePolicy  command.CreatePolicyHandler
	UpdatePolicy  command.UpdatePolicyHandler
	DeletePolicy  command.DeletePolicyHandler
	EnablePolicy  command.EnablePolicyHandler
	DisablePolicy command.DisablePolicyHandler
}

// Queries read policies.
type Queries struct {
	GetPolicy    query.GetPolicyHandler
	ListPolicies query.ListPoliciesHandler
}

// New wires handlers. Both dependencies are required.
func New(deps Dependencies) Application {
	if deps.Policies == nil || deps.Resource == nil || deps.Metrics == nil {
		panic("application.New: policies, resource, and metrics are required")
	}
	return Application{
		Commands: Commands{
			CreatePolicy:  command.NewCreatePolicyHandler(deps.Policies, deps.Metrics),
			UpdatePolicy:  command.NewUpdatePolicyHandler(deps.Policies, deps.Resource, deps.Metrics),
			DeletePolicy:  command.NewDeletePolicyHandler(deps.Policies, deps.Metrics),
			EnablePolicy:  command.NewEnablePolicyHandler(deps.Policies, deps.Resource, deps.Metrics),
			DisablePolicy: command.NewDisablePolicyHandler(deps.Policies, deps.Metrics),
		},
		Queries: Queries{
			GetPolicy:    query.NewGetPolicyHandler(deps.Policies, deps.Metrics),
			ListPolicies: query.NewListPoliciesHandler(deps.Policies, deps.Metrics),
		},
	}
}
