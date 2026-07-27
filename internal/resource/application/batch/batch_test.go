package batch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

func TestCompensateCreated(t *testing.T) {
	t.Parallel()

	cause := errors.New("root")
	if got := compensateCreated(context.Background(), "t", nil, func(context.Context, string, []string) error {
		t.Fatal("should not delete")
		return nil
	}, cause); !errors.Is(got, cause) {
		t.Fatalf("empty ids: %v", got)
	}
	if got := compensateCreated(context.Background(), "t", []string{"a"}, nil, cause); !errors.Is(got, cause) {
		t.Fatalf("nil deleteFn: %v", got)
	}
	if got := compensateCreated(context.Background(), "t", []string{"a"}, func(context.Context, string, []string) error {
		return nil
	}, nil); got != nil {
		t.Fatalf("nil cause: %v", got)
	}

	called := false
	got := compensateCreated(context.Background(), "t", []string{"a", "b"}, func(_ context.Context, tenant string, ids []string) error {
		called = true
		if tenant != "t" || len(ids) != 2 {
			t.Fatalf("tenant=%s ids=%v", tenant, ids)
		}
		return nil
	}, cause)
	if !called || !errors.Is(got, cause) {
		t.Fatalf("success path: called=%v err=%v", called, got)
	}

	got = compensateCreated(context.Background(), "t", []string{"a"}, func(context.Context, string, []string) error {
		return errors.New("delete boom")
	}, cause)
	if !errors.Is(got, cause) || !strings.Contains(got.Error(), "compensate delete") {
		t.Fatalf("compensate fail wrap: %v", got)
	}
}

type stubCURepo struct {
	createCalls int
	createErrAt int // 1-based; 0 = never fail
	created     [][]*model.CU
	deleted     []string
	deleteErr   error
}

func (r *stubCURepo) Create(context.Context, *model.CU) (*model.CU, error) {
	return nil, errors.New("not implemented")
}
func (r *stubCURepo) Update(context.Context, *model.CU) error { return errors.New("not implemented") }
func (r *stubCURepo) FindByID(context.Context, string, string) (*model.CU, error) {
	return nil, errors.New("not implemented")
}
func (r *stubCURepo) List(context.Context, port.CUFilter) (*port.PageResult[*model.CU], error) {
	return nil, errors.New("not implemented")
}

func (r *stubCURepo) BatchCreate(_ context.Context, cus []*model.CU) error {
	r.createCalls++
	cp := append([]*model.CU(nil), cus...)
	r.created = append(r.created, cp)
	if r.createErrAt > 0 && r.createCalls == r.createErrAt {
		return errors.New("insert fail")
	}
	return nil
}

func (r *stubCURepo) BatchDelete(_ context.Context, _ string, ids []string) error {
	r.deleted = append(r.deleted, ids...)
	return r.deleteErr
}

func TestBatchCreateCUs_ValidationAndCompensate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("empty tenant", func(t *testing.T) {
		t.Parallel()
		_, err := BatchCreateCUs(ctx, &stubCURepo{}, "  ", nil, 10, nil)
		if err == nil {
			t.Fatal("want tenant error")
		}
	})

	t.Run("item validation", func(t *testing.T) {
		t.Parallel()
		repo := &stubCURepo{}
		_, err := BatchCreateCUs(ctx, repo, "t", []types.CUItem{
			{Name: "ok", Type: "x"},
			{Name: "", Type: ""},
		}, 10, nil)
		if err == nil || !errors.Is(err, types.ErrCUBatchValidation) {
			t.Fatalf("want validation error, got %v", err)
		}
		if repo.createCalls != 0 {
			t.Fatal("should not create on validation failure")
		}
	})

	t.Run("second chunk fails then compensates", func(t *testing.T) {
		t.Parallel()
		repo := &stubCURepo{createErrAt: 2}
		parent := "asset-1"
		items := []types.CUItem{
			{ParentID: &parent, Name: "cu-1", Type: "modbus"},
			{ParentID: &parent, Name: "cu-2", Type: "modbus"},
		}
		ids, err := BatchCreateCUs(ctx, repo, "tenant", items, 1, nil)
		if err == nil || ids != nil {
			t.Fatalf("want error with nil ids, got ids=%v err=%v", ids, err)
		}
		if !strings.Contains(err.Error(), "batch insert") {
			t.Fatalf("err = %v", err)
		}
		if repo.createCalls != 2 {
			t.Fatalf("createCalls = %d", repo.createCalls)
		}
		if len(repo.deleted) != 1 {
			t.Fatalf("deleted = %v, want 1 id from first chunk", repo.deleted)
		}
		if len(repo.created[0]) != 1 {
			t.Fatal("first chunk size")
		}
		if repo.deleted[0] != repo.created[0][0].ID {
			t.Fatalf("compensate id mismatch: deleted=%s created=%s", repo.deleted[0], repo.created[0][0].ID)
		}
	})

	t.Run("onChunk error compensates", func(t *testing.T) {
		t.Parallel()
		repo := &stubCURepo{}
		parent := "asset-1"
		_, err := BatchCreateCUs(ctx, repo, "tenant", []types.CUItem{
			{ParentID: &parent, Name: "cu-1", Type: "modbus"},
		}, 10, func(int) error { return errors.New("progress save failed") })
		if err == nil || !strings.Contains(err.Error(), "progress save failed") {
			t.Fatalf("err = %v", err)
		}
		if len(repo.deleted) != 1 {
			t.Fatalf("deleted = %v", repo.deleted)
		}
	})

	t.Run("compensate delete failure wraps", func(t *testing.T) {
		t.Parallel()
		repo := &stubCURepo{createErrAt: 2, deleteErr: errors.New("delete boom")}
		parent := "asset-1"
		_, err := BatchCreateCUs(ctx, repo, "tenant", []types.CUItem{
			{ParentID: &parent, Name: "cu-1", Type: "modbus"},
			{ParentID: &parent, Name: "cu-2", Type: "modbus"},
		}, 1, nil)
		if err == nil || !strings.Contains(err.Error(), "compensate delete") {
			t.Fatalf("err = %v", err)
		}
	})
}

type stubPointRepo struct {
	deleted   [][]string
	failChunk int // 1-based chunk index to fail; 0 = never
	calls     int
}

func (r *stubPointRepo) Create(context.Context, *model.Point) (*model.Point, error) {
	return nil, errors.New("not implemented")
}
func (r *stubPointRepo) BatchCreate(context.Context, []*model.Point) error {
	return errors.New("not implemented")
}
func (r *stubPointRepo) Update(context.Context, *model.Point) error { return errors.New("not implemented") }
func (r *stubPointRepo) FindByID(context.Context, string, string) (*model.Point, error) {
	return nil, errors.New("not implemented")
}
func (r *stubPointRepo) List(context.Context, port.PointFilter) (*port.PageResult[*model.Point], error) {
	return nil, errors.New("not implemented")
}
func (r *stubPointRepo) SoftDelete(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (r *stubPointRepo) BatchDelete(_ context.Context, _ string, ids []string) error {
	r.calls++
	r.deleted = append(r.deleted, append([]string(nil), ids...))
	if r.failChunk > 0 && r.calls == r.failChunk {
		return errors.New("delete chunk fail")
	}
	return nil
}

func TestBatchDeletePoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("empty tenant", func(t *testing.T) {
		t.Parallel()
		if err := BatchDeletePoints(ctx, &stubPointRepo{}, "", []string{"a"}, 10, nil); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("empty ids", func(t *testing.T) {
		t.Parallel()
		repo := &stubPointRepo{}
		if err := BatchDeletePoints(ctx, repo, "t", nil, 10, nil); err != nil {
			t.Fatal(err)
		}
		if repo.calls != 0 {
			t.Fatal("no calls expected")
		}
	})

	t.Run("partial failure continues", func(t *testing.T) {
		t.Parallel()
		repo := &stubPointRepo{failChunk: 1}
		err := BatchDeletePoints(ctx, repo, "t", []string{"a", "b", "c"}, 2, nil)
		var partial *BatchDeletePointsPartialError
		if !errors.As(err, &partial) {
			t.Fatalf("want partial error, got %v", err)
		}
		if partial.Succeeded != 1 || len(partial.FailedIDs) != 2 {
			t.Fatalf("succeeded=%d failedIDs=%v", partial.Succeeded, partial.FailedIDs)
		}
		if repo.calls != 2 {
			t.Fatalf("calls = %d, want 2 (continue after fail)", repo.calls)
		}
	})

	t.Run("onChunk aborts", func(t *testing.T) {
		t.Parallel()
		repo := &stubPointRepo{}
		err := BatchDeletePoints(ctx, repo, "t", []string{"a", "b"}, 1, func(int) error {
			return errors.New("abort")
		})
		if err == nil || !strings.Contains(err.Error(), "abort") {
			t.Fatalf("err = %v", err)
		}
		if repo.calls != 1 {
			t.Fatalf("calls = %d", repo.calls)
		}
	})
}
