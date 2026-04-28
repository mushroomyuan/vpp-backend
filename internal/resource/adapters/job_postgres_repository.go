package adapters

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	"gorm.io/gorm"
)

type JobRepositoryPostgres struct {
	repo *postgres.JobRepository
}

func NewJobRepositoryPostgres(repo *postgres.JobRepository) *JobRepositoryPostgres {
	return &JobRepositoryPostgres{repo: repo}
}

var _ port.JobRepository = (*JobRepositoryPostgres)(nil)

func (r *JobRepositoryPostgres) Create(ctx context.Context, job *model.Job) error {
	return r.repo.CreateJob(ctx, jobDomainToDB(job))
}

func (r *JobRepositoryPostgres) FindByID(ctx context.Context, id string) (*model.Job, error) {
	row, err := r.repo.FindJobByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	return jobDBToDomain(row), nil
}

func (r *JobRepositoryPostgres) ClaimPending(ctx context.Context) (*model.Job, error) {
	row, err := r.repo.ClaimPendingJob(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return jobDBToDomain(row), nil
}

func (r *JobRepositoryPostgres) Save(ctx context.Context, job *model.Job) error {
	return r.repo.SaveJob(ctx, jobDomainToDB(job))
}
