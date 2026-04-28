package application

import (
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"github.com/mushroomyuan/vpp-backend/resource/application/worker"
	"github.com/mushroomyuan/vpp-backend/resource/application/worker/executors"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type Application struct {
	Commands Commands
	Queries  Queries
	Workers  Workers
}

type Commands struct {
	// Site
	CreateSite command.CreateSiteHandler
	UpdateSite command.UpdateSiteHandler
	DeleteSite command.DeleteSiteHandler

	// Resource
	CreateResource command.CreateResourceHandler
	UpdateResource command.UpdateResourceHandler
	DeleteResource command.DeleteResourceHandler

	// CU
	CreateCU command.CreateCUHandler
	UpdateCU command.UpdateCUHandler
	DeleteCU command.DeleteCUHandler

	// Point
	CreatePoint command.CreatePointHandler
	UpdatePoint command.UpdatePointHandler
	DeletePoint command.DeletePointHandler

	// Job
	SubmitBatchImport command.SubmitBatchImportHandler
	SubmitBatchDelete command.SubmitBatchDeleteHandler
	RetryJob          command.RetryJobHandler
}

type Queries struct {
	// Site
	GetSite   query.GetSiteHandler
	ListSites query.ListSitesHandler

	// Resource
	GetResource   query.GetResourceHandler
	ListResources query.ListResourcesHandler

	// CU
	GetCU   query.GetCUHandler
	ListCUs query.ListCUsHandler

	// Point
	GetPoint   query.GetPointHandler
	ListPoints query.ListPointsHandler

	// Job
	GetJob query.GetJobHandler
}

type Workers struct {
	ImportWorker *worker.ImportWorker
	Executors    worker.ExecutorRegistry
}

type Dependencies struct {
	// Repositories (ports)
	SiteRepo     port.SiteRepository
	ResourceRepo port.ResourceRepository
	CURepo       port.CURepository
	PointRepo    port.PointRepository
	JobRepo      port.JobRepository

	// Cross-cutting
	Metrics decorator.MetricsClient

	// Worker
	ImportWorkerConfig worker.ImportWorkerConfig
}

func NewApplication(deps Dependencies) Application {
	workerRegistry := worker.ExecutorRegistry{
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetResource}: executors.NewResourceImportExecutor(deps.ResourceRepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetCU}:       executors.NewCUImportExecutor(deps.CURepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetPoint}:    executors.NewPointImportExecutor(deps.PointRepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationDelete, Target: model.JobTargetResource}: executors.NewResourceDeleteExecutor(deps.ResourceRepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationDelete, Target: model.JobTargetCU}:       executors.NewCUDeleteExecutor(deps.CURepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationDelete, Target: model.JobTargetPoint}:    executors.NewPointDeleteExecutor(deps.PointRepo, deps.JobRepo),
	}

	return Application{
		Commands: Commands{
			// Site
			CreateSite: command.NewCreateSiteHandler(deps.SiteRepo, deps.Metrics),
			UpdateSite: command.NewUpdateSiteHandler(deps.SiteRepo, deps.Metrics),
			DeleteSite: command.NewDeleteSiteHandler(deps.SiteRepo, deps.Metrics),

			// Resource
			CreateResource: command.NewCreateResourceHandler(deps.ResourceRepo, deps.Metrics),
			UpdateResource: command.NewUpdateResourceHandler(deps.ResourceRepo, deps.Metrics),
			DeleteResource: command.NewDeleteResourceHandler(deps.ResourceRepo, deps.Metrics),

			// CU
			CreateCU: command.NewCreateCUHandler(deps.CURepo, deps.Metrics),
			UpdateCU: command.NewUpdateCUHandler(deps.CURepo, deps.Metrics),
			DeleteCU: command.NewDeleteCUHandler(deps.CURepo, deps.Metrics),

			// Point
			CreatePoint: command.NewCreatePointHandler(deps.PointRepo, deps.Metrics),
			UpdatePoint: command.NewUpdatePointHandler(deps.PointRepo, deps.Metrics),
			DeletePoint: command.NewDeletePointHandler(deps.PointRepo, deps.Metrics),

			// Job
			SubmitBatchImport: command.NewSubmitBatchImportHandler(deps.JobRepo, deps.Metrics),
			SubmitBatchDelete: command.NewSubmitBatchDeleteHandler(deps.JobRepo, deps.Metrics),
			RetryJob:          command.NewRetryJobHandler(deps.JobRepo, deps.Metrics),
		},
		Queries: Queries{
			// Site
			GetSite:   query.NewGetSiteHandler(deps.SiteRepo, deps.Metrics),
			ListSites: query.NewListSitesHandler(deps.SiteRepo, deps.Metrics),

			// Resource
			GetResource:   query.NewGetResourceHandler(deps.ResourceRepo, deps.Metrics),
			ListResources: query.NewListResourcesHandler(deps.ResourceRepo, deps.Metrics),

			// CU
			GetCU:   query.NewGetCUHandler(deps.CURepo, deps.Metrics),
			ListCUs: query.NewListCUsHandler(deps.CURepo, deps.Metrics),

			// Point
			GetPoint:   query.NewGetPointHandler(deps.PointRepo, deps.Metrics),
			ListPoints: query.NewListPointsHandler(deps.PointRepo, deps.Metrics),

			// Job
			GetJob: query.NewGetJobHandler(deps.JobRepo, deps.Metrics),
		},
		Workers: Workers{
			Executors:    workerRegistry,
			ImportWorker: worker.NewImportWorker(deps.JobRepo, workerRegistry, deps.ImportWorkerConfig),
		},
	}
}
