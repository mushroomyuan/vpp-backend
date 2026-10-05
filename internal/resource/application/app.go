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
	CreateCU           command.CreateCUHandler
	UpdateCU           command.UpdateCUHandler
	CreateCUCapability command.CreateCUCapabilityHandler
	UpdateCUCapability command.UpdateCUCapabilityHandler
	DeleteCUCapability command.DeleteCUCapabilityHandler

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
	GetCU              query.GetCUHandler
	ListCUs            query.ListCUsHandler
	GetCUCapability    query.GetCUCapabilityHandler
	ListCUCapabilities query.ListCUCapabilitiesHandler

	// Point
	GetPoint   query.GetPointHandler
	ListPoints query.ListPointsHandler

	// Scope
	ResolveScope query.ResolveScopeHandler

	// Job
	GetJob query.GetJobHandler
}

type Workers struct {
	ImportWorker *worker.ImportWorker
	Executors    worker.ExecutorRegistry
}

type Dependencies struct {
	// Repositories (ports)
	SiteRepo         port.SiteRepository
	AssetRepo        port.AssetRepository
	CURepo           port.CURepository
	CUCapabilityRepo port.CUCapabilityRepository
	PointRepo        port.PointRepository
	JobRepo          port.JobRepository
	NodeRepo         port.NodeRepository
	ScopeRepo        port.ScopeRepository

	// Cross-cutting
	Metrics decorator.MetricsClient

	// Worker
	ImportWorkerConfig worker.ImportWorkerConfig

	// Event bus — nil-safe: when not set, all Publish calls are no-ops inside handlers.
	EventPublisher port.ResourceEventPublisher
}

func NewApplication(deps Dependencies) Application {
	if deps.NodeRepo == nil {
		panic("NewApplication: NodeRepo is required")
	}
	if deps.CUCapabilityRepo == nil {
		panic("NewApplication: CUCapabilityRepo is required")
	}
	if deps.ScopeRepo == nil {
		panic("NewApplication: ScopeRepo is required")
	}

	pub := deps.EventPublisher // may be nil; handlers guard with nil check

	workerRegistry := worker.ExecutorRegistry{
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetAsset}: executors.NewAssetImportExecutor(deps.AssetRepo, deps.JobRepo, pub),
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetCU}:    executors.NewCUImportExecutor(deps.CURepo, deps.JobRepo, pub),
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetPoint}: executors.NewPointImportExecutor(deps.PointRepo, deps.JobRepo, pub),
		model.JobKind{Operation: model.JobOperationDelete, Target: model.JobTargetPoint}: executors.NewPointDeleteExecutor(deps.PointRepo, deps.JobRepo),
	}

	return Application{
		Commands: Commands{
			// Site
			CreateSite: command.NewCreateSiteHandler(deps.SiteRepo, deps.Metrics, pub),
			UpdateSite: command.NewUpdateSiteHandler(deps.SiteRepo, deps.Metrics, pub),

			// Asset
			CreateAsset: command.NewCreateAssetHandler(deps.AssetRepo, deps.Metrics, pub),
			UpdateAsset: command.NewUpdateAssetHandler(deps.AssetRepo, deps.Metrics, pub),
			// Node
			DeleteResource:          command.NewDeleteResourceHandler(deps.NodeRepo, deps.Metrics, pub),
			MoveResource:            command.NewMoveResourceHandler(deps.NodeRepo, deps.Metrics),
			BatchMoveResources:      command.NewBatchMoveResourcesHandler(deps.NodeRepo, deps.Metrics),
			RenameResource:          command.NewRenameResourceHandler(deps.NodeRepo, deps.Metrics, pub),
			ChangeResourceLifecycle: command.NewChangeResourceLifecycleHandler(deps.NodeRepo, deps.Metrics, pub),

			// CU
			CreateCU:           command.NewCreateCUHandler(deps.CURepo, deps.NodeRepo, deps.Metrics, pub),
			UpdateCU:           command.NewUpdateCUHandler(deps.CURepo, deps.NodeRepo, deps.Metrics, pub),
			CreateCUCapability: command.NewCreateCUCapabilityHandler(deps.CUCapabilityRepo, deps.CURepo, deps.Metrics),
			UpdateCUCapability: command.NewUpdateCUCapabilityHandler(deps.CUCapabilityRepo, deps.Metrics),
			DeleteCUCapability: command.NewDeleteCUCapabilityHandler(deps.CUCapabilityRepo, deps.Metrics),

			// Point
			CreatePoint: command.NewCreatePointHandler(deps.PointRepo, deps.NodeRepo, deps.Metrics, pub),
			UpdatePoint: command.NewUpdatePointHandler(deps.PointRepo, deps.Metrics, pub),
			DeletePoint: command.NewDeletePointHandler(deps.PointRepo, deps.Metrics, pub),

			// Job
			SubmitBatchImport: command.NewSubmitBatchImportHandler(deps.JobRepo, deps.Metrics),
			RetryJob:          command.NewRetryJobHandler(deps.JobRepo, deps.Metrics),
		},
		Queries: Queries{
			// Site
			GetSite:   query.NewGetSiteHandler(deps.SiteRepo, deps.Metrics),
			ListSites: query.NewListSitesHandler(deps.SiteRepo, deps.Metrics),

			// Asset
			GetAsset:           query.NewGetAssetHandler(deps.AssetRepo, deps.Metrics),
			ListAssets:         query.NewListAssetsHandler(deps.AssetRepo, deps.Metrics),
			GetResourceDetail:  query.NewGetResourceDetailHandler(deps.NodeRepo, deps.Metrics),
			ListChildren:       query.NewListChildrenHandler(deps.NodeRepo, deps.Metrics),
			GetBreadcrumb:      query.NewGetBreadcrumbHandler(deps.NodeRepo, deps.Metrics),
			ExportResourceTree: query.NewExportResourceTreeHandler(deps.NodeRepo, deps.Metrics),

			// CU
			GetCU:              query.NewGetCUHandler(deps.CURepo, deps.Metrics),
			ListCUs:            query.NewListCUsHandler(deps.CURepo, deps.Metrics),
			GetCUCapability:    query.NewGetCUCapabilityHandler(deps.CUCapabilityRepo, deps.Metrics),
			ListCUCapabilities: query.NewListCUCapabilitiesHandler(deps.CUCapabilityRepo, deps.Metrics),

			// Point
			GetPoint:   query.NewGetPointHandler(deps.PointRepo, deps.Metrics),
			ListPoints: query.NewListPointsHandler(deps.PointRepo, deps.Metrics),

			ResolveScope: query.NewResolveScopeHandler(deps.ScopeRepo, deps.Metrics),

			// Job
			GetJob: query.NewGetJobHandler(deps.JobRepo, deps.Metrics),
		},
		Workers: Workers{
			Executors:    workerRegistry,
			ImportWorker: worker.NewImportWorker(deps.JobRepo, workerRegistry, deps.ImportWorkerConfig),
		},
	}
}
