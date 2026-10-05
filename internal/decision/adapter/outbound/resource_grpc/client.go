// Package resourcegrpc implements port.ResourcePort with ResourceService.ResolveScope.
package resourcegrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream resource service.
type Config struct {
	Addr        string
	DialOptions []grpc.DialOption
	Timeout     time.Duration
	Breaker     *gobreaker.CircuitBreaker[any]
}

// Client calls ResourceService.ResolveScope.
type Client struct {
	client resourcepb.ResourceServiceClient
	conn   *grpc.ClientConn
}

var _ port.ResourcePort = (*Client)(nil)

// NewClient dials resource. The caller must Close on shutdown.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("resource_grpc: addr is required")
	}
	conn, err := dial(cfg.Addr, cfg.DialOptions, cfg.Timeout, cfg.Breaker)
	if err != nil {
		return nil, fmt.Errorf("resource_grpc: %w", err)
	}
	logrus.Infof("resource_grpc: dialed %s", cfg.Addr)
	return &Client{client: resourcepb.NewResourceServiceClient(conn), conn: conn}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// ResolveScope expands a site, asset, or CU and returns members, exclusions, and the revision.
func (c *Client) ResolveScope(ctx context.Context, query port.ScopeQuery) (port.ResolvedScope, error) {
	scopeType, err := scopeTypeToProto(query.ScopeType)
	if err != nil {
		return port.ResolvedScope{}, err
	}
	if query.TenantID == "" {
		return port.ResolvedScope{}, fmt.Errorf("resource_grpc: tenant id is required")
	}
	if query.ScopeID == "" {
		return port.ResolvedScope{}, fmt.Errorf("resource_grpc: scope id is required")
	}
	metrics := make([]*resourcepb.MetricRequirement, len(query.RequiredMetrics))
	for i, req := range query.RequiredMetrics {
		access, err := metricAccessToProto(req.Access)
		if err != nil {
			return port.ResolvedScope{}, err
		}
		if req.MetricID == "" {
			return port.ResolvedScope{}, fmt.Errorf("resource_grpc: required metric id is empty")
		}
		metrics[i] = &resourcepb.MetricRequirement{
			MetricID: string(req.MetricID),
			Access:   access,
		}
	}
	caps := make([]string, len(query.RequiredCapabilityIDs))
	for i, id := range query.RequiredCapabilityIDs {
		if id == "" {
			return port.ResolvedScope{}, fmt.Errorf("resource_grpc: required capability id is empty")
		}
		caps[i] = string(id)
	}

	resp, err := c.client.ResolveScope(ctx, &resourcepb.ResolveScopeRequest{
		TenantID:              query.TenantID,
		ScopeType:             scopeType,
		ScopeID:               query.ScopeID,
		RequiredCapabilityIDs: caps,
		RequiredMetrics:       metrics,
	})
	if err != nil {
		return port.ResolvedScope{}, fmt.Errorf("resource_grpc: ResolveScope: %w", err)
	}
	return scopeFromProto(resp)
}

func scopeFromProto(resp *resourcepb.ResolveScopeResponse) (port.ResolvedScope, error) {
	if resp == nil {
		return port.ResolvedScope{}, fmt.Errorf("resource_grpc: empty ResolveScope response")
	}
	scopeType, err := scopeTypeFromProto(resp.GetScopeType())
	if err != nil {
		return port.ResolvedScope{}, err
	}
	members := make([]port.ResolvedCU, 0, len(resp.GetMembers()))
	for _, member := range resp.GetMembers() {
		if member == nil {
			continue
		}
		members = append(members, cuFromProto(member))
	}
	exclusions := make([]port.ScopeExclusion, 0, len(resp.GetExclusions()))
	for _, item := range resp.GetExclusions() {
		if item == nil {
			continue
		}
		exclusions = append(exclusions, port.ScopeExclusion{
			CUID:    item.GetCUID(),
			AssetID: item.GetAssetID(),
			Reason:  exclusionFromProto(item.GetReason()),
			Detail:  item.GetDetail(),
		})
	}
	failures := make([]port.ScopePrecheckFailure, 0, len(resp.GetPrecheckFailures()))
	for _, item := range resp.GetPrecheckFailures() {
		if item == nil {
			continue
		}
		failures = append(failures, port.ScopePrecheckFailure{
			CUID:    item.GetCUID(),
			AssetID: item.GetAssetID(),
			Reason:  precheckFromProto(item.GetReason()),
			Detail:  item.GetDetail(),
		})
	}
	return port.ResolvedScope{
		ScopeType:        scopeType,
		ScopeID:          resp.GetScopeID(),
		ResourceRevision: resp.GetResourceRevision(),
		PrecheckOK:       resp.GetPrecheckOK(),
		Members:          members,
		Exclusions:       exclusions,
		PrecheckFailures: failures,
	}, nil
}

func cuFromProto(cu *resourcepb.ResolvedCU) port.ResolvedCU {
	caps := make([]port.ResolvedCapability, 0, len(cu.GetCapabilities()))
	for _, cap := range cu.GetCapabilities() {
		if cap == nil {
			continue
		}
		var spec map[string]any
		if raw := cap.GetSpec(); raw != nil {
			spec = raw.AsMap()
		}
		caps = append(caps, port.ResolvedCapability{
			CapabilityID:  contracts.CapabilityID(cap.GetCapabilityID()),
			SchemaVersion: cap.GetSchemaVersion(),
			Spec:          spec,
			Enabled:       cap.GetEnabled(),
			Version:       cap.GetVersion(),
		})
	}
	bindings := make([]port.ResolvedBinding, 0, len(cu.GetBindings()))
	for _, binding := range cu.GetBindings() {
		if binding == nil {
			continue
		}
		bindings = append(bindings, port.ResolvedBinding{
			MetricID:   contracts.MetricID(binding.GetMetricID()),
			AccessMode: bindingAccessFromProto(binding.GetAccessMode()),
			Enabled:    binding.GetEnabled(),
			Revision:   binding.GetRevision(),
			Safety:     safetyFromProto(binding.GetSafetyConstraint()),
		})
	}
	return port.ResolvedCU{
		CUID:         cu.GetCUID(),
		AssetID:      cu.GetAssetID(),
		Lifecycle:    lifecycleFromProto(cu.GetLifecycleStatus()),
		NodeVersion:  cu.GetNodeVersion(),
		Capabilities: caps,
		Bindings:     bindings,
	}
}

func safetyFromProto(in *resourcepb.PointSafetyConstraint) *port.SafetyConstraint {
	if in == nil {
		return nil
	}
	out := &port.SafetyConstraint{Version: in.GetVersion()}
	if min := in.GetMinValue(); min != nil {
		v := min.GetValue()
		out.MinValue = &v
	}
	if max := in.GetMaxValue(); max != nil {
		v := max.GetValue()
		out.MaxValue = &v
	}
	if rate := in.GetMaxChangePerSecond(); rate != nil {
		v := rate.GetValue()
		out.MaxChangePerSecond = &v
	}
	return out
}

func scopeTypeToProto(t port.ScopeType) (resourcepb.ScopeType, error) {
	switch t {
	case port.ScopeSite:
		return resourcepb.ScopeType_SCOPE_TYPE_SITE, nil
	case port.ScopeAsset:
		return resourcepb.ScopeType_SCOPE_TYPE_ASSET, nil
	case port.ScopeCU:
		return resourcepb.ScopeType_SCOPE_TYPE_CU, nil
	default:
		return 0, fmt.Errorf("resource_grpc: unsupported scope type %q", t)
	}
}

func scopeTypeFromProto(t resourcepb.ScopeType) (port.ScopeType, error) {
	switch t {
	case resourcepb.ScopeType_SCOPE_TYPE_SITE:
		return port.ScopeSite, nil
	case resourcepb.ScopeType_SCOPE_TYPE_ASSET:
		return port.ScopeAsset, nil
	case resourcepb.ScopeType_SCOPE_TYPE_CU:
		return port.ScopeCU, nil
	default:
		return "", fmt.Errorf("resource_grpc: unsupported scope type %s", t.String())
	}
}

func metricAccessToProto(access port.MetricAccess) (resourcepb.MetricAccessRequirement, error) {
	switch access {
	case port.MetricAccessRead:
		return resourcepb.MetricAccessRequirement_METRIC_ACCESS_REQUIREMENT_READ, nil
	case port.MetricAccessWrite:
		return resourcepb.MetricAccessRequirement_METRIC_ACCESS_REQUIREMENT_WRITE, nil
	default:
		return 0, fmt.Errorf("resource_grpc: unsupported metric access %q", access)
	}
}

func bindingAccessFromProto(mode resourcepb.PointAccessMode) port.BindingAccess {
	switch mode {
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ:
		return port.BindingAccessRead
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_WRITE:
		return port.BindingAccessWrite
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ_WRITE:
		return port.BindingAccessReadWrite
	default:
		return port.BindingAccessUnspecified
	}
}

func lifecycleFromProto(status resourcepb.ResourceLifecycleStatus) port.Lifecycle {
	switch status {
	case resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_ACTIVE:
		return port.LifecycleActive
	case resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_DISABLED:
		return port.LifecycleDisabled
	case resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_DECOMMISSIONED:
		return port.LifecycleDecommissioned
	default:
		return port.LifecycleUnspecified
	}
}

func exclusionFromProto(reason resourcepb.ScopeExclusionReason) port.ExclusionReason {
	switch reason {
	case resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_NOT_ACTIVE:
		return port.ExclusionNotActive
	case resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_MISSING_CAPABILITY:
		return port.ExclusionMissingCapability
	case resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_CAPABILITY_DISABLED:
		return port.ExclusionCapabilityDisabled
	default:
		return port.ExclusionUnspecified
	}
}

func precheckFromProto(reason resourcepb.ScopePrecheckFailureReason) port.PrecheckFailureReason {
	switch reason {
	case resourcepb.ScopePrecheckFailureReason_SCOPE_PRECHECK_FAILURE_REASON_INVALID_CAPABILITY_SPEC:
		return port.PrecheckInvalidCapabilitySpec
	case resourcepb.ScopePrecheckFailureReason_SCOPE_PRECHECK_FAILURE_REASON_MISSING_METRIC_BINDING:
		return port.PrecheckMissingMetricBinding
	default:
		return port.PrecheckUnspecified
	}
}

func dial(addr string, extra []grpc.DialOption, timeout time.Duration, breaker *gobreaker.CircuitBreaker[any]) (*grpc.ClientConn, error) {
	var interceptors []grpc.UnaryClientInterceptor
	if timeout > 0 {
		interceptors = append(interceptors, resilience.UnaryClientTimeoutInterceptor(timeout))
	}
	if breaker != nil {
		interceptors = append(interceptors, resilience.UnaryClientBreakerInterceptor(breaker))
	}
	opts := extra
	if len(interceptors) > 0 {
		opts = append([]grpc.DialOption{grpc.WithChainUnaryInterceptor(interceptors...)}, extra...)
	}
	return platformserver.DialGRPC(addr, opts...)
}
