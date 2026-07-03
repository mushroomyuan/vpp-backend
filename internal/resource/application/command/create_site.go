package command

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreateSite struct {
	TenantID    string
	Name        string
	Location    model.Location
	Description string
}

type CreateSiteResult struct {
	SiteID string
}

type CreateSiteHandler decorator.CommandHandler[CreateSite, *CreateSiteResult]

type createSiteHandler struct {
	siteRepo  port.SiteRepository
	publisher port.ResourceEventPublisher
}

func NewCreateSiteHandler(
	siteRepo port.SiteRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) CreateSiteHandler {
	if siteRepo == nil {
		panic("NewCreateSiteHandler parameter siteRepo is nil")
	}
	return decorator.ApplyCommandDecorators[CreateSite, *CreateSiteResult](
		createSiteHandler{
			siteRepo:  siteRepo,
			publisher: publisher,
		},
		metricClient,
	)
}

func (c createSiteHandler) Handle(ctx context.Context, cmd CreateSite) (*CreateSiteResult, error) {
	ctx, span := telemetry.Start(ctx, "create_site")
	defer span.End()

	id := idgen.Must()

	var desc *string
	if trimmed := strings.TrimSpace(cmd.Description); trimmed != "" {
		desc = &trimmed
	}
	var loc *model.Location
	if cmd.Location != (model.Location{}) {
		l := cmd.Location
		loc = &l
	}

	site, err := model.NewSite(model.CreateSiteParams{
		ID:              id,
		TenantID:        cmd.TenantID,
		ParentID:        nil,
		DisplayName:     strings.TrimSpace(cmd.Name),
		Description:     desc,
		Location:        loc,
		OperatingStatus: model.OperatingStatusUnderConstruction,
		SubType:         nil,
	})
	if err != nil {
		return nil, err
	}

	if _, err := c.siteRepo.Create(ctx, site); err != nil {
		return nil, err
	}

	if c.publisher != nil {
		descStr := ""
		if desc != nil {
			descStr = *desc
		}
		if pubErr := c.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeSiteCreated,
			TenantID:   cmd.TenantID,
			ResourceID: id,
			Payload: platEvent.SiteCreatedPayload{
				SiteID:      id,
				TenantID:    cmd.TenantID,
				Name:        strings.TrimSpace(cmd.Name),
				Description: descStr,
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish site created event")
		}
	}

	return &CreateSiteResult{SiteID: id}, nil
}
