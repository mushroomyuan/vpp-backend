package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	gatewaycommand "github.com/mushroomyuan/vpp-backend/gateway/application/command"
	gatewayquery "github.com/mushroomyuan/vpp-backend/gateway/application/query"
	gatewaymodel "github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	resourceport "github.com/mushroomyuan/vpp-backend/resource/domain/port"

	resEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
)

// TestResourceLifecycle_DisablesGatewayMapping exercises the cross-service
// chain: resource service publishes a lifecycle event on vpp.resource.events
// -> gateway's LifecycleConsumer (started in buildEnv) consumes it -> drives
// DisableMappingByCUCode -> the mapping's Status flips to disabled.
//
// The "resource service" side is played by ResourceEvents, a real
// resource/adapter/outbound/kafka.EventPublisher pointed at the shared Kafka
// container — the same production code resource's command handlers use —
// without needing to stand up resource's full domain/hierarchy (which would
// require a pre-existing CU row for NodeRepository.UpdateStatus).
func TestResourceLifecycle_DisablesGatewayMapping(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()

	const tenantID = "tenant-lifecycle"
	const cuCode = "cu-lifecycle-001"

	_, err := e.Gateway.Commands.CreateMapping.Handle(ctx, gatewaycommand.CreateMapping{
		TenantID:       tenantID,
		ExternalSystem: "ems-test",
		ExternalID:     "device-lifecycle-1",
		CUCode:         cuCode,
	})
	require.NoError(t, err, "seed active device mapping")

	err = e.ResourceEvents.Publish(ctx, resourceport.ResourceEvent{
		EventType:  resEvent.TypeResourceDeleted,
		TenantID:   tenantID,
		ResourceID: cuCode,
		Payload: resEvent.ResourceDeletedPayload{
			ResourceID: cuCode,
			TenantID:   tenantID,
		},
	})
	require.NoError(t, err, "publish resource.deleted event")

	var mappingStatus gatewaymodel.MappingStatus
	requireEventuallyf(t, func() bool {
		res, qErr := e.Gateway.Queries.ListMappings.Handle(ctx, gatewayquery.ListMappings{TenantID: tenantID})
		if qErr != nil {
			return false
		}
		for _, m := range res.Mappings {
			if m.CUCode == cuCode {
				mappingStatus = m.Status
				return mappingStatus == gatewaymodel.MappingStatusDisabled
			}
		}
		return false
	}, "mapping for %s was never disabled after resource.deleted", cuCode)

	require.Equal(t, gatewaymodel.MappingStatusDisabled, mappingStatus)
}

// TestResourceLifecycle_LifecycleChangedDisablesMapping covers the second
// event type the consumer reacts to: resource.lifecycle.changed with a
// terminal Status (archived), as opposed to a hard resource.deleted event.
func TestResourceLifecycle_LifecycleChangedDisablesMapping(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()

	const tenantID = "tenant-lifecycle-archived"
	const cuCode = "cu-lifecycle-002"

	_, err := e.Gateway.Commands.CreateMapping.Handle(ctx, gatewaycommand.CreateMapping{
		TenantID:       tenantID,
		ExternalSystem: "ems-test",
		ExternalID:     "device-lifecycle-2",
		CUCode:         cuCode,
	})
	require.NoError(t, err, "seed active device mapping")

	err = e.ResourceEvents.Publish(ctx, resourceport.ResourceEvent{
		EventType:  resEvent.TypeLifecycleChanged,
		TenantID:   tenantID,
		ResourceID: cuCode,
		Payload: resEvent.LifecycleChangedPayload{
			ResourceID: cuCode,
			TenantID:   tenantID,
			Status:     "archived",
		},
	})
	require.NoError(t, err, "publish resource.lifecycle.changed event")

	requireEventuallyf(t, func() bool {
		res, qErr := e.Gateway.Queries.ListMappings.Handle(ctx, gatewayquery.ListMappings{TenantID: tenantID})
		if qErr != nil {
			return false
		}
		for _, m := range res.Mappings {
			if m.CUCode == cuCode {
				return m.Status == gatewaymodel.MappingStatusDisabled
			}
		}
		return false
	}, "mapping for %s was never disabled after resource.lifecycle.changed", cuCode)
}
