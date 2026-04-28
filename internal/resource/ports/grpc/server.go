package grpc

import (
	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// Server implements resourcepb.ResourceServiceServer.
// Handlers are sourced from the pre-wired application.Application (CQRS layer),
// which eliminates duplicate handler construction.
// The three repos are kept as direct fields because the batch gRPC APIs
// (BatchCreateResources / BatchCreateCUs / BatchCreatePoints) call repo methods
// in a tight in-process loop rather than going through a command handler, in
// order to stream progress back to the caller per-chunk.
type Server struct {
	resourcepb.UnimplementedResourceServiceServer

	// repos used directly by batch gRPC handlers
	resourceRepo port.ResourceRepository
	cuRepo       port.CURepository
	pointRepo    port.PointRepository

	// CQRS handlers (command side)
	createSite        command.CreateSiteHandler
	updateSite        command.UpdateSiteHandler
	deleteSite        command.DeleteSiteHandler
	createResource    command.CreateResourceHandler
	updateResource    command.UpdateResourceHandler
	deleteResource    command.DeleteResourceHandler
	submitBatchImport command.SubmitBatchImportHandler
	submitBatchDelete command.SubmitBatchDeleteHandler
	createCU          command.CreateCUHandler
	updateCU          command.UpdateCUHandler
	deleteCU          command.DeleteCUHandler
	createPoint       command.CreatePointHandler
	updatePoint       command.UpdatePointHandler
	deletePoint       command.DeletePointHandler
	retryJob          command.RetryJobHandler

	// CQRS handlers (query side)
	getSite       query.GetSiteHandler
	listSites     query.ListSitesHandler
	getResource   query.GetResourceHandler
	listResources query.ListResourcesHandler
	getCU         query.GetCUHandler
	listCUs       query.ListCUsHandler
	getPoint      query.GetPointHandler
	listPoints    query.ListPointsHandler
	getJob        query.GetJobHandler
}

// NewServer constructs a Server from a fully-wired application.Application.
// The three repositories are required for batch gRPC operations that bypass the
// command handler layer for performance reasons; all other operations are served
// via the pre-built handlers inside app.Commands and app.Queries.
func NewServer(
	app application.Application,
	resourceRepo port.ResourceRepository,
	cuRepo port.CURepository,
	pointRepo port.PointRepository,
) *Server {
	if resourceRepo == nil {
		panic("NewServer: resourceRepo must not be nil")
	}
	if cuRepo == nil {
		panic("NewServer: cuRepo must not be nil")
	}
	if pointRepo == nil {
		panic("NewServer: pointRepo must not be nil")
	}

	return &Server{
		// direct repos for batch handlers
		resourceRepo: resourceRepo,
		cuRepo:       cuRepo,
		pointRepo:    pointRepo,

		// command handlers
		createSite:        app.Commands.CreateSite,
		updateSite:        app.Commands.UpdateSite,
		deleteSite:        app.Commands.DeleteSite,
		createResource:    app.Commands.CreateResource,
		updateResource:    app.Commands.UpdateResource,
		deleteResource:    app.Commands.DeleteResource,
		submitBatchImport: app.Commands.SubmitBatchImport,
		submitBatchDelete: app.Commands.SubmitBatchDelete,
		createCU:          app.Commands.CreateCU,
		updateCU:          app.Commands.UpdateCU,
		deleteCU:          app.Commands.DeleteCU,
		createPoint:       app.Commands.CreatePoint,
		updatePoint:       app.Commands.UpdatePoint,
		deletePoint:       app.Commands.DeletePoint,
		retryJob:          app.Commands.RetryJob,

		// query handlers
		getSite:       app.Queries.GetSite,
		listSites:     app.Queries.ListSites,
		getResource:   app.Queries.GetResource,
		listResources: app.Queries.ListResources,
		getCU:         app.Queries.GetCU,
		listCUs:       app.Queries.ListCUs,
		getPoint:      app.Queries.GetPoint,
		listPoints:    app.Queries.ListPoints,
		getJob:        app.Queries.GetJob,
	}
}
