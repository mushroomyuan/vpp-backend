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

	// Asset (legacy API name: resource)
	CreateAsset command.CreateAssetHandler
	UpdateAsset command.UpdateAssetHandler
	// Node
	DeleteResource          command.DeleteResourceHandler
	MoveResource            command.MoveResourceHandler
	BatchMoveResources      command.BatchMoveResourcesHandler
	RenameResource          command.RenameResourceHandler
	ChangeResourceLifecycle command.ChangeResourceLifecycleHandler

	// CU
	CreateCU command.CreateCUHandler
	UpdateCU command.UpdateCUHandler

	// Point
	CreatePoint command.CreatePointHandler
	UpdatePoint command.UpdatePointHandler
	DeletePoint command.DeletePointHandler

	// Job
	SubmitBatchImport command.SubmitBatchImportHandler
	RetryJob          command.RetryJobHandler
}

type Queries struct {
	// Site
	GetSite   query.GetSiteHandler
	ListSites query.ListSitesHandler

	// Asset
	GetAsset           query.GetAssetHandler
	ListAssets         query.ListAssetsHandler
	GetResourceDetail  query.GetResourceDetailHandler
	ListChildren       query.ListChildrenHandler
	GetBreadcrumb      query.GetBreadcrumbHandler
	ExportResourceTree query.ExportResourceTreeHandler

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
	SiteRepo  port.SiteRepository
	AssetRepo port.AssetRepository
	CURepo    port.CURepository
	PointRepo port.PointRepository
	JobRepo   port.JobRepository
	NodeRepo  port.NodeRepository

	// Runtime readers (Redis-backed hot state)
	AssetRuntime port.AssetRuntimeReader
	CURuntime    port.CURuntimeReader
	PointRuntime port.PointRuntimeReader

	// Cross-cutting
	Metrics decorator.MetricsClient

	// Worker
	ImportWorkerConfig worker.ImportWorkerConfig
}

func NewApplication(deps Dependencies) Application {
	if deps.NodeRepo == nil {
		panic("NewApplication: NodeRepo is required")
	}
	if deps.AssetRuntime == nil {
		panic("NewApplication: AssetRuntime is required")
	}
	if deps.CURuntime == nil {
		panic("NewApplication: CURuntime is required")
	}
	if deps.PointRuntime == nil {
		panic("NewApplication: PointRuntime is required")
	}
	workerRegistry := worker.ExecutorRegistry{
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetAsset}: executors.NewAssetImportExecutor(deps.AssetRepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetCU}:    executors.NewCUImportExecutor(deps.CURepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetPoint}: executors.NewPointImportExecutor(deps.PointRepo, deps.JobRepo),
		model.JobKind{Operation: model.JobOperationDelete, Target: model.JobTargetPoint}: executors.NewPointDeleteExecutor(deps.PointRepo, deps.JobRepo),
	}

	return Application{
		Commands: Commands{
			// Site
			CreateSite: command.NewCreateSiteHandler(deps.SiteRepo, deps.Metrics),
			UpdateSite: command.NewUpdateSiteHandler(deps.SiteRepo, deps.Metrics),

			// Asset
			CreateAsset: command.NewCreateAssetHandler(deps.AssetRepo, deps.Metrics),
			UpdateAsset: command.NewUpdateAssetHandler(deps.AssetRepo, deps.Metrics),
			// Node
			DeleteResource:          command.NewDeleteResourceHandler(deps.NodeRepo, deps.Metrics),
			MoveResource:            command.NewMoveResourceHandler(deps.NodeRepo, deps.Metrics),
			BatchMoveResources:      command.NewBatchMoveResourcesHandler(deps.NodeRepo, deps.Metrics),
			RenameResource:          command.NewRenameResourceHandler(deps.NodeRepo, deps.Metrics),
			ChangeResourceLifecycle: command.NewChangeResourceLifecycleHandler(deps.NodeRepo, deps.Metrics),

			// CU
			CreateCU: command.NewCreateCUHandler(deps.CURepo, deps.NodeRepo, deps.Metrics),
			UpdateCU: command.NewUpdateCUHandler(deps.CURepo, deps.NodeRepo, deps.Metrics),

			// Point
			CreatePoint: command.NewCreatePointHandler(deps.PointRepo, deps.NodeRepo, deps.Metrics),
			UpdatePoint: command.NewUpdatePointHandler(deps.PointRepo, deps.Metrics),
			DeletePoint: command.NewDeletePointHandler(deps.PointRepo, deps.Metrics),

			// Job
			SubmitBatchImport: command.NewSubmitBatchImportHandler(deps.JobRepo, deps.Metrics),
			RetryJob:          command.NewRetryJobHandler(deps.JobRepo, deps.Metrics),
		},
		Queries: Queries{
			// Site
			GetSite:   query.NewGetSiteHandler(deps.SiteRepo, deps.Metrics),
			ListSites: query.NewListSitesHandler(deps.SiteRepo, deps.Metrics),

			// Asset
			GetAsset:           query.NewGetAssetHandler(deps.AssetRepo, deps.AssetRuntime, deps.Metrics),
			ListAssets:         query.NewListAssetsHandler(deps.AssetRepo, deps.AssetRuntime, deps.Metrics),
			GetResourceDetail:  query.NewGetResourceDetailHandler(deps.NodeRepo, deps.Metrics),
			ListChildren:       query.NewListChildrenHandler(deps.NodeRepo, deps.Metrics),
			GetBreadcrumb:      query.NewGetBreadcrumbHandler(deps.NodeRepo, deps.Metrics),
			ExportResourceTree: query.NewExportResourceTreeHandler(deps.NodeRepo, deps.Metrics),

			// CU
			GetCU:   query.NewGetCUHandler(deps.CURepo, deps.CURuntime, deps.Metrics),
			ListCUs: query.NewListCUsHandler(deps.CURepo, deps.CURuntime, deps.Metrics),

			// Point
			GetPoint:   query.NewGetPointHandler(deps.PointRepo, deps.PointRuntime, deps.Metrics),
			ListPoints: query.NewListPointsHandler(deps.PointRepo, deps.PointRuntime, deps.Metrics),

			// Job
			GetJob: query.NewGetJobHandler(deps.JobRepo, deps.Metrics),
		},
		Workers: Workers{
			Executors:    workerRegistry,
			ImportWorker: worker.NewImportWorker(deps.JobRepo, workerRegistry, deps.ImportWorkerConfig),
		},
	}
}
