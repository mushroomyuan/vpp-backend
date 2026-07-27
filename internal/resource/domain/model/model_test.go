package model

import (
	"strings"
	"testing"
	"time"
)

func TestNewJob(t *testing.T) {
	t.Parallel()

	t.Run("rejects missing fields", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			id   string
			tid  string
			op   JobOperationType
			tgt  JobTargetType
			pay  []byte
		}{
			{"empty id", "", "t", JobOperationImport, JobTargetCU, []byte(`{}`)},
			{"empty tenant", "j", "", JobOperationImport, JobTargetCU, []byte(`{}`)},
			{"empty op", "j", "t", "", JobTargetCU, []byte(`{}`)},
			{"empty target", "j", "t", JobOperationImport, "", []byte(`{}`)},
			{"empty payload", "j", "t", JobOperationImport, JobTargetCU, nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if _, err := NewJob(tc.id, tc.tid, tc.op, tc.tgt, tc.pay); err == nil {
					t.Fatal("want error")
				}
			})
		}
	})

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		j, err := NewJob("j1", "tenant", JobOperationImport, JobTargetAsset, []byte(`{"x":1}`))
		if err != nil {
			t.Fatal(err)
		}
		if j.Status != JobStatusPending {
			t.Fatalf("status = %q", j.Status)
		}
		if j.MaxAttempts != defaultMaxAttempts {
			t.Fatalf("MaxAttempts = %d", j.MaxAttempts)
		}
		if j.Kind() != (JobKind{Operation: JobOperationImport, Target: JobTargetAsset}) {
			t.Fatalf("Kind = %+v", j.Kind())
		}
	})
}

func TestJob_Start(t *testing.T) {
	t.Parallel()

	j, err := NewJob("j1", "t", JobOperationImport, JobTargetCU, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Start(); err != nil {
		t.Fatal(err)
	}
	if j.Status != JobStatusRunning || j.Attempts != 1 || j.StartedAt == nil {
		t.Fatalf("after Start: status=%q attempts=%d started=%v", j.Status, j.Attempts, j.StartedAt)
	}
	if err := j.Start(); err == nil {
		t.Fatal("second Start should fail")
	}
}

func TestJob_CompleteAndFail(t *testing.T) {
	t.Parallel()

	j, _ := NewJob("j1", "t", JobOperationImport, JobTargetPoint, []byte(`{}`))
	_ = j.Start()
	j.Complete(3, 2, 1, []byte(`{"ok":true}`))
	if j.Status != JobStatusSuccess {
		t.Fatalf("status = %q", j.Status)
	}
	if j.Total != 3 || j.Succeeded != 2 || j.FailedCount != 1 {
		t.Fatalf("counts %d/%d/%d", j.Total, j.Succeeded, j.FailedCount)
	}
	if string(j.ResultJSON) != `{"ok":true}` || j.FinishedAt == nil || j.NextRetryAt != nil {
		t.Fatal("Complete fields incomplete")
	}

	j2, _ := NewJob("j2", "t", JobOperationImport, JobTargetCU, []byte(`{}`))
	_ = j2.Start()
	before := time.Now()
	j2.Fail("boom")
	if j2.Status != JobStatusFailed || j2.ErrorMsg != "boom" || j2.FinishedAt == nil {
		t.Fatal("Fail fields incomplete")
	}
	if j2.NextRetryAt == nil || j2.NextRetryAt.Before(before.Add(retryBackoffAfterFail-time.Second)) {
		t.Fatalf("NextRetryAt = %v", j2.NextRetryAt)
	}
}

func TestJob_CanRetryAndReset(t *testing.T) {
	t.Parallel()

	j, _ := NewJob("j1", "t", JobOperationImport, JobTargetCU, []byte(`{}`))
	if j.CanRetry() {
		t.Fatal("pending should not retry")
	}
	_ = j.Start()
	j.Fail("x")
	if !j.CanRetry() {
		t.Fatal("failed with attempts < max should retry")
	}
	if err := j.ResetForRetry(); err != nil {
		t.Fatal(err)
	}
	if j.Status != JobStatusPending || j.ErrorMsg != "" || j.StartedAt != nil || j.FinishedAt != nil || j.NextRetryAt != nil {
		t.Fatalf("ResetForRetry cleared incompletely: %+v", j)
	}

	j.Attempts = j.MaxAttempts
	j.Status = JobStatusFailed
	if j.CanRetry() {
		t.Fatal("at max attempts should not retry")
	}
	if err := j.ResetForRetry(); err == nil {
		t.Fatal("ResetForRetry at max should fail")
	}
}

func TestJob_UpdateProgress(t *testing.T) {
	t.Parallel()
	j, _ := NewJob("j1", "t", JobOperationImport, JobTargetCU, []byte(`{}`))
	j.UpdateProgress(4, 1)
	if j.Succeeded != 4 || j.FailedCount != 1 {
		t.Fatalf("progress = %d/%d", j.Succeeded, j.FailedCount)
	}
}

func TestNode_IdentityHelpers(t *testing.T) {
	t.Parallel()

	if err := ValidateCreateNodeIdentity("", "t", "n"); err == nil {
		t.Fatal("want id error")
	}
	if err := ValidateCreateNodeIdentity("id", " ", "n"); err == nil {
		t.Fatal("want tenant error")
	}
	if err := ValidateCreateNodeIdentity("id", "t", "  "); err == nil {
		t.Fatal("want name error")
	}
	if err := ValidateCreateNodeIdentity("id", "t", "n"); err != nil {
		t.Fatal(err)
	}

	if NormalizeParentID("  ") != nil {
		t.Fatal("blank parent should be nil")
	}
	p := NormalizeParentID(" parent ")
	if p == nil || *p != "parent" {
		t.Fatalf("got %v", p)
	}
	blank := "  "
	if NormalizeParentIDPtr(&blank) != nil {
		t.Fatal("blank ptr should be nil")
	}
	if NormalizeParentIDPtr(nil) != nil {
		t.Fatal("nil ptr should stay nil")
	}
}

func TestNode_RenameAndLifecycle(t *testing.T) {
	t.Parallel()

	n := &Node{DisplayName: "old", Version: 1, LifecycleStatus: NodeLifecycleDraft}
	if err := n.Rename("  "); err == nil {
		t.Fatal("empty rename")
	}
	if err := n.Rename("  new "); err != nil {
		t.Fatal(err)
	}
	if n.DisplayName != "new" || n.Version != 2 {
		t.Fatalf("rename result name=%q ver=%d", n.DisplayName, n.Version)
	}

	n.UpdateDescription("  ")
	if n.Description != nil {
		t.Fatal("empty desc should clear")
	}
	n.UpdateDescription(" hi ")
	if n.Description == nil || *n.Description != "hi" {
		t.Fatal("desc not set")
	}

	if err := n.UpdateLifecycleStatus(NodeLifecycleDeleted); err == nil {
		t.Fatal("deleted not allowed via UpdateLifecycleStatus")
	}
	if err := n.UpdateLifecycleStatus(NodeLifecycleActive); err != nil {
		t.Fatal(err)
	}
	if n.LifecycleStatus != NodeLifecycleActive {
		t.Fatal("lifecycle not updated")
	}
}

func TestNode_SoftDeleteRestore(t *testing.T) {
	t.Parallel()

	n := &Node{LifecycleStatus: NodeLifecycleActive, Version: 1}
	if err := n.SoftDelete(); err == nil || !strings.Contains(err.Error(), "disable") {
		t.Fatalf("active delete: %v", err)
	}
	n.LifecycleStatus = NodeLifecycleDisabled
	if err := n.SoftDelete(); err != nil {
		t.Fatal(err)
	}
	if !n.IsDeleted() || n.IsActive() {
		t.Fatal("expected deleted inactive")
	}
	if err := n.SoftDelete(); err == nil {
		t.Fatal("already deleted")
	}
	if err := n.Restore(); err != nil {
		t.Fatal(err)
	}
	if n.IsDeleted() {
		t.Fatal("restored should not be deleted")
	}
	if err := n.Restore(); err == nil {
		t.Fatal("restore when not deleted")
	}
}

func TestNode_Metadata(t *testing.T) {
	t.Parallel()
	n := &Node{}
	if _, ok := n.GetMetadata("k"); ok {
		t.Fatal("missing key")
	}
	n.SetMetadata("k", 42)
	v, ok := n.GetMetadata("k")
	if !ok || v != 42 {
		t.Fatalf("got %v %v", v, ok)
	}
}

func TestNewPoint(t *testing.T) {
	t.Parallel()

	if _, err := NewPoint(CreatePointParams{}); err == nil {
		t.Fatal("want validation error")
	}
	p, err := NewPoint(CreatePointParams{
		ID: "p1", AssetID: "a1", CUID: "c1", PointKey: "soc", DataType: DataTypeFloat,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ExtConfig == nil || p.SafetyThresholds == nil {
		t.Fatal("maps should be initialized")
	}
	if !DataTypeFloat.IsValid() || DataType("x").IsValid() {
		t.Fatal("DataType.IsValid")
	}
}

func TestNewAsset(t *testing.T) {
	t.Parallel()

	if _, err := NewAsset(CreateAssetParams{}); err == nil {
		t.Fatal("want identity error")
	}
	a, err := NewAsset(CreateAssetParams{
		ID: "a1", TenantID: "t", DisplayName: "BESS",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Type != NodeTypeAsset || a.DispatchStatus != DispatchStatusUnknown {
		t.Fatalf("type=%q status=%q", a.Type, a.DispatchStatus)
	}
}
