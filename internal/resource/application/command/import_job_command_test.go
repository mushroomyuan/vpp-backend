package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type stubJobRepo struct {
	created []*model.Job
	byID    map[string]*model.Job
	createErr error
	saveErr   error
	saved     []*model.Job
}

func (r *stubJobRepo) Create(_ context.Context, job *model.Job) error {
	if r.createErr != nil {
		return r.createErr
	}
	cp := *job
	r.created = append(r.created, &cp)
	if r.byID == nil {
		r.byID = map[string]*model.Job{}
	}
	r.byID[job.ID] = &cp
	return nil
}

func (r *stubJobRepo) FindByID(_ context.Context, id string) (*model.Job, error) {
	if j, ok := r.byID[id]; ok {
		cp := *j
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (r *stubJobRepo) ClaimPending(context.Context) (*model.Job, error) {
	return nil, errors.New("not implemented")
}

func (r *stubJobRepo) Save(_ context.Context, job *model.Job) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *job
	r.saved = append(r.saved, &cp)
	if r.byID != nil {
		r.byID[job.ID] = &cp
	}
	return nil
}

func TestSubmitBatchImport_ValidationRejectsWithoutCreate(t *testing.T) {
	t.Parallel()
	repo := &stubJobRepo{}
	h := submitBatchImportHandler{jobRepo: repo}

	res, err := h.Handle(context.Background(), SubmitBatchImport{
		CU: &types.CUImportSpec{
			TenantID: "t1",
			CUImportPayload: types.CUImportPayload{
				Items: []types.CUItem{{Name: "", Type: ""}},
			},
		},
	})
	if !errors.Is(err, types.ErrBatchImportValidation) {
		t.Fatalf("err = %v", err)
	}
	if res == nil || len(res.FailedItems) == 0 {
		t.Fatal("want FailedItems")
	}
	if len(repo.created) != 0 {
		t.Fatal("must not Create job on validation failure")
	}
}

func TestSubmitBatchImport_CreatesJob(t *testing.T) {
	t.Parallel()
	repo := &stubJobRepo{}
	h := submitBatchImportHandler{jobRepo: repo}

	res, err := h.Handle(context.Background(), SubmitBatchImport{
		CU: &types.CUImportSpec{
			TenantID: "tenant-1",
			CUImportPayload: types.CUImportPayload{
				BatchSize: 50,
				Items:     []types.CUItem{{Name: "cu-1", Type: "modbus"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.JobID == "" {
		t.Fatal("empty JobID")
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %d", len(repo.created))
	}
	j := repo.created[0]
	if j.TenantID != "tenant-1" || j.OperationType != model.JobOperationImport || j.TargetType != model.JobTargetCU {
		t.Fatalf("job fields: %+v", j)
	}
	if j.Status != model.JobStatusPending || len(j.Payload) == 0 {
		t.Fatal("pending job with payload expected")
	}
}

func TestSubmitBatchImport_RequiresOneof(t *testing.T) {
	t.Parallel()
	h := submitBatchImportHandler{jobRepo: &stubJobRepo{}}
	_, err := h.Handle(context.Background(), SubmitBatchImport{})
	if err == nil {
		t.Fatal("want error when no target set")
	}
}

func TestSubmitBatchImport_AssetAndPoint(t *testing.T) {
	t.Parallel()

	t.Run("asset", func(t *testing.T) {
		t.Parallel()
		repo := &stubJobRepo{}
		h := submitBatchImportHandler{jobRepo: repo}
		res, err := h.Handle(context.Background(), SubmitBatchImport{
			Asset: &types.AssetImportSpec{
				TenantID: "t",
				AssetImportPayload: types.AssetImportPayload{
					SiteID: "s1",
					Items:  []types.AssetItem{{Name: "a1"}},
				},
			},
		})
		if err != nil || res.JobID == "" {
			t.Fatalf("err=%v res=%+v", err, res)
		}
		if repo.created[0].TargetType != model.JobTargetAsset {
			t.Fatal(repo.created[0].TargetType)
		}
	})

	t.Run("point", func(t *testing.T) {
		t.Parallel()
		repo := &stubJobRepo{}
		h := submitBatchImportHandler{jobRepo: repo}
		res, err := h.Handle(context.Background(), SubmitBatchImport{
			Point: &types.PointImportSpec{
				TenantID: "t",
				PointImportPayload: types.PointImportPayload{
					AssetID: "a1",
					CUID:    "c1",
					Items:   []types.PointItem{{PointKey: "soc", DataType: model.DataTypeFloat}},
				},
			},
		})
		if err != nil || res.JobID == "" {
			t.Fatalf("err=%v res=%+v", err, res)
		}
		if repo.created[0].TargetType != model.JobTargetPoint {
			t.Fatal(repo.created[0].TargetType)
		}
	})
}

func TestRetryJob(t *testing.T) {
	t.Parallel()

	now := time.Now()
	failed := &model.Job{
		ID:          "job-1",
		TenantID:    "t",
		Status:      model.JobStatusFailed,
		Attempts:    1,
		MaxAttempts: 3,
		ErrorMsg:    "boom",
		FinishedAt:  &now,
		NextRetryAt: &now,
	}
	repo := &stubJobRepo{byID: map[string]*model.Job{"job-1": failed}}
	h := retryJobHandler{jobRepo: repo}

	if _, err := h.Handle(context.Background(), RetryJob{JobID: "job-1"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.saved) != 1 {
		t.Fatal("want Save")
	}
	if repo.saved[0].Status != model.JobStatusPending || repo.saved[0].ErrorMsg != "" {
		t.Fatalf("saved = %+v", repo.saved[0])
	}

	exhausted := &model.Job{
		ID: "job-2", Status: model.JobStatusFailed, Attempts: 3, MaxAttempts: 3,
	}
	repo2 := &stubJobRepo{byID: map[string]*model.Job{"job-2": exhausted}}
	h2 := retryJobHandler{jobRepo: repo2}
	if _, err := h2.Handle(context.Background(), RetryJob{JobID: "job-2"}); err == nil {
		t.Fatal("want error when attempts exhausted")
	}
}
