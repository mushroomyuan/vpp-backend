package grpc

import (
	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
)

// Server implements resourcepb.ResourceServiceServer.
// Handlers are sourced from the pre-wired application.Application (CQRS layer),
// which eliminates duplicate handler construction.
type Server struct {
	resourcepb.UnimplementedResourceServiceServer

	// CQRS handlers (command side)
	createSite        command.CreateSiteHandler
	updateSite        command.UpdateSiteHandler
	createAsset       command.CreateAssetHandler
	updateAsset       command.UpdateAssetHandler
	deleteResource    command.DeleteResourceHandler
	moveResource      command.MoveResourceHandler
	batchMoveResource command.BatchMoveResourcesHandler
	renameResource    command.RenameResourceHandler
	changeLifecycle   command.ChangeResourceLifecycleHandler
	submitBatchImport command.SubmitBatchImportHandler
	createCU          command.CreateCUHandler
	updateCU          command.UpdateCUHandler
	createPoint       command.CreatePointHandler
	updatePoint       command.UpdatePointHandler
	deletePoint       command.DeletePointHandler
	retryJob          command.RetryJobHandler

	// CQRS handlers (query side)
	getSite           query.GetSiteHandler
	listSites         query.ListSitesHandler
	getAsset          query.GetAssetHandler
	listAssets        query.ListAssetsHandler
	getResourceDetail query.GetResourceDetailHandler
	listChildren      query.ListChildrenHandler
	getBreadcrumb     query.GetBreadcrumbHandler
	exportTree        query.ExportResourceTreeHandler
	getCU             query.GetCUHandler
	listCUs           query.ListCUsHandler
	getPoint          query.GetPointHandler
	listPoints        query.ListPointsHandler
	getJob            query.GetJobHandler
}

// NewServer constructs a Server from a fully-wired application.Application.
func NewServer(app application.Application) *Server {
	return &Server{
		// command handlers
		createSite:        app.Commands.CreateSite,
		updateSite:        app.Commands.UpdateSite,
		createAsset:       app.Commands.CreateAsset,
		updateAsset:       app.Commands.UpdateAsset,
		deleteResource:    app.Commands.DeleteResource,
		moveResource:      app.Commands.MoveResource,
		batchMoveResource: app.Commands.BatchMoveResources,
		renameResource:    app.Commands.RenameResource,
		changeLifecycle:   app.Commands.ChangeResourceLifecycle,
		submitBatchImport: app.Commands.SubmitBatchImport,
		createCU:          app.Commands.CreateCU,
		updateCU:          app.Commands.UpdateCU,
		createPoint:       app.Commands.CreatePoint,
		updatePoint:       app.Commands.UpdatePoint,
		deletePoint:       app.Commands.DeletePoint,
		retryJob:          app.Commands.RetryJob,

		// query handlers
		getSite:           app.Queries.GetSite,
		listSites:         app.Queries.ListSites,
		getAsset:          app.Queries.GetAsset,
		listAssets:        app.Queries.ListAssets,
		getResourceDetail: app.Queries.GetResourceDetail,
		listChildren:      app.Queries.ListChildren,
		getBreadcrumb:     app.Queries.GetBreadcrumb,
		exportTree:        app.Queries.ExportResourceTree,
		getCU:             app.Queries.GetCU,
		listCUs:           app.Queries.ListCUs,
		getPoint:          app.Queries.GetPoint,
		listPoints:        app.Queries.ListPoints,
		getJob:            app.Queries.GetJob,
	}
}
