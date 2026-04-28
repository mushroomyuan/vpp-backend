package worker

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

// Executor carries out the actual import logic for a specific job type.
// Each implementation receives the claimed job (whose Payload contains
// the type-specific input) and returns the JSON-encoded result to be stored
// in import_jobs.result_json on success.
type Executor interface {
	Execute(ctx context.Context, job *model.Job) (resultJSON []byte, err error)
}

// ExecutorRegistry maps job types to their concrete executors (instances, not
// factories). Executors hold only shared, stateless service dependencies
// (repos, etc.) and receive all per-execution state via Execute's ctx and job
// parameters. Creating a new instance per job would add overhead without any
// benefit; a factory would be appropriate only if executors needed per-call
// setup such as acquiring a dedicated connection.
type ExecutorRegistry map[model.JobKind]Executor

func (r ExecutorRegistry) Get(kind model.JobKind) (Executor, error) {
	e, ok := r[kind]
	if !ok {
		return nil, fmt.Errorf("no executor registered for job kind %+v", kind)
	}
	return e, nil
}
