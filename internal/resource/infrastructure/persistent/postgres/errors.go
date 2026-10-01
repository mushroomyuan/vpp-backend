package postgres

import "errors"

var ErrOptimisticLock = errors.New("optimistic lock conflict")
