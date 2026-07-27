package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type stubJobRepo struct {
	mu sync.Mutex

	claimJob *model.Job
	claimErr error

	saved   []*model.Job
	saveErr error
}

func (r *stubJobRepo) Create(context.Context, *model.Job) error { return nil }

func (r *stubJobRepo) FindByID(context.Context, string) (*model.Job, error) {
	return nil, errors.New("not implemented")
}

func (r *stubJobRepo) ClaimPending(context.Context) (*model.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	return r.claimJob, nil
}

func (r *stubJobRepo) Save(_ context.Context, job *model.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *job
	r.saved = append(r.saved, &cp)
	return nil
}

func (r *stubJobRepo) lastSaved() *model.Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.saved) == 0 {
		return nil
	}
	return r.saved[len(r.saved)-1]
}

type stubExecutor struct {
	result []byte
	err    error
	calls  int
}

func (e *stubExecutor) Execute(context.Context, *model.Job) ([]byte, error) {
	e.calls++
	return e.result, e.err
}

func runningJob(op model.JobOperationType, target model.JobTargetType) *model.Job {
	now := time.Now()
	return &model.Job{
		ID:            "job-1",
		TenantID:      "tenant-1",
		OperationType: op,
		TargetType:    target,
		Status:        model.JobStatusRunning,
		Payload:       []byte(`{}`),
		Total:         2,
		Succeeded:     2,
		FailedCount:   0,
		Attempts:      1,
		MaxAttempts:   3,
		StartedAt:     &now,
	}
}

func TestNewImportWorker_DefaultPollInterval(t *testing.T) {
	w := NewImportWorker(&stubJobRepo{}, ExecutorRegistry{}, ImportWorkerConfig{})
	if w.interval != defaultPollInterval {
		t.Fatalf("interval = %v, want %v", w.interval, defaultPollInterval)
	}
}

func TestNewImportWorker_CustomPollInterval(t *testing.T) {
	w := NewImportWorker(&stubJobRepo{}, ExecutorRegistry{}, ImportWorkerConfig{
		PollInterval: 200 * time.Millisecond,
	})
	if w.interval != 200*time.Millisecond {
		t.Fatalf("interval = %v, want 200ms", w.interval)
	}
}

func TestExecutorRegistry_Get(t *testing.T) {
	kind := model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetCU}
	reg := ExecutorRegistry{kind: &stubExecutor{}}

	if _, err := reg.Get(kind); err != nil {
		t.Fatalf("Get registered: %v", err)
	}
	if _, err := reg.Get(model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetAsset}); err == nil {
		t.Fatal("Get missing kind: want error")
	}
}

func TestProcessNext_NoPendingJob(t *testing.T) {
	repo := &stubJobRepo{}
	w := NewImportWorker(repo, ExecutorRegistry{}, ImportWorkerConfig{PollInterval: time.Millisecond})

	w.processNext(context.Background())

	if repo.lastSaved() != nil {
		t.Fatal("expected no Save when queue empty")
	}
}

func TestProcessNext_ClaimError(t *testing.T) {
	repo := &stubJobRepo{claimErr: errors.New("db down")}
	w := NewImportWorker(repo, ExecutorRegistry{}, ImportWorkerConfig{PollInterval: time.Millisecond})

	w.processNext(context.Background())

	if repo.lastSaved() != nil {
		t.Fatal("expected no Save when ClaimPending fails")
	}
}

func TestProcessNext_Success(t *testing.T) {
	job := runningJob(model.JobOperationImport, model.JobTargetCU)
	repo := &stubJobRepo{claimJob: job}
	exec := &stubExecutor{result: []byte(`{"cu_ids":["a"]}`)}
	reg := ExecutorRegistry{
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetCU}: exec,
	}
	w := NewImportWorker(repo, reg, ImportWorkerConfig{PollInterval: time.Millisecond})

	w.processNext(context.Background())

	if exec.calls != 1 {
		t.Fatalf("Execute calls = %d, want 1", exec.calls)
	}
	saved := repo.lastSaved()
	if saved == nil {
		t.Fatal("expected Save after success")
	}
	if saved.Status != model.JobStatusSuccess {
		t.Fatalf("status = %q, want %q", saved.Status, model.JobStatusSuccess)
	}
	if string(saved.ResultJSON) != `{"cu_ids":["a"]}` {
		t.Fatalf("result_json = %s", saved.ResultJSON)
	}
	if saved.Total != 2 || saved.Succeeded != 2 {
		t.Fatalf("counts total=%d succeeded=%d", saved.Total, saved.Succeeded)
	}
	if saved.FinishedAt == nil {
		t.Fatal("FinishedAt should be set")
	}
}

func TestProcessNext_NoExecutor(t *testing.T) {
	job := runningJob(model.JobOperationImport, model.JobTargetCU)
	repo := &stubJobRepo{claimJob: job}
	w := NewImportWorker(repo, ExecutorRegistry{}, ImportWorkerConfig{PollInterval: time.Millisecond})

	w.processNext(context.Background())

	saved := repo.lastSaved()
	if saved == nil {
		t.Fatal("expected Save after missing executor")
	}
	if saved.Status != model.JobStatusFailed {
		t.Fatalf("status = %q, want %q", saved.Status, model.JobStatusFailed)
	}
	if saved.ErrorMsg == "" {
		t.Fatal("ErrorMsg should be set")
	}
	if saved.NextRetryAt == nil {
		t.Fatal("NextRetryAt should be set on Fail")
	}
}

func TestProcessNext_ExecutorError(t *testing.T) {
	job := runningJob(model.JobOperationImport, model.JobTargetAsset)
	repo := &stubJobRepo{claimJob: job}
	exec := &stubExecutor{err: errors.New("insert failed")}
	reg := ExecutorRegistry{
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetAsset}: exec,
	}
	w := NewImportWorker(repo, reg, ImportWorkerConfig{PollInterval: time.Millisecond})

	w.processNext(context.Background())

	if exec.calls != 1 {
		t.Fatalf("Execute calls = %d, want 1", exec.calls)
	}
	saved := repo.lastSaved()
	if saved == nil {
		t.Fatal("expected Save after executor error")
	}
	if saved.Status != model.JobStatusFailed {
		t.Fatalf("status = %q, want %q", saved.Status, model.JobStatusFailed)
	}
	if saved.ErrorMsg != "insert failed" {
		t.Fatalf("ErrorMsg = %q, want insert failed", saved.ErrorMsg)
	}
}

func TestProcessNext_CompleteSaveError(t *testing.T) {
	job := runningJob(model.JobOperationImport, model.JobTargetPoint)
	repo := &stubJobRepo{
		claimJob: job,
		saveErr:  errors.New("save failed"),
	}
	exec := &stubExecutor{result: []byte(`{}`)}
	reg := ExecutorRegistry{
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetPoint}: exec,
	}
	w := NewImportWorker(repo, reg, ImportWorkerConfig{PollInterval: time.Millisecond})

	w.processNext(context.Background())

	if exec.calls != 1 {
		t.Fatalf("Execute calls = %d, want 1", exec.calls)
	}
	// Save failed: nothing persisted, but in-memory job should already be Complete.
	if job.Status != model.JobStatusSuccess {
		t.Fatalf("in-memory status = %q, want %q", job.Status, model.JobStatusSuccess)
	}
	if repo.lastSaved() != nil {
		t.Fatal("stub should not record Save when saveErr is set")
	}
}

func TestStart_StopsOnCancel(t *testing.T) {
	repo := &stubJobRepo{}
	w := NewImportWorker(repo, ExecutorRegistry{}, ImportWorkerConfig{
		PollInterval: 20 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestStart_ProcessesClaimedJob(t *testing.T) {
	job := runningJob(model.JobOperationImport, model.JobTargetCU)
	repo := &stubJobRepo{claimJob: job}
	exec := &stubExecutor{result: []byte(`{"ok":true}`)}
	reg := ExecutorRegistry{
		model.JobKind{Operation: model.JobOperationImport, Target: model.JobTargetCU}: exec,
	}
	w := NewImportWorker(repo, reg, ImportWorkerConfig{
		PollInterval: 15 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if exec.calls >= 1 && repo.lastSaved() != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job processing")
		case <-time.After(10 * time.Millisecond):
		}
	}

	saved := repo.lastSaved()
	if saved.Status != model.JobStatusSuccess {
		t.Fatalf("status = %q, want %q", saved.Status, model.JobStatusSuccess)
	}
	cancel()
}
