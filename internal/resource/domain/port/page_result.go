package port

type PageResult[T any] struct {
	Items      []T
	TotalCount int64
	Offset     int
	Limit      int
}
