package factory

import "sync"

type Supplier func(string) any

type Singleton struct {
	supplier Supplier
	cache    map[string]any
	locker   *sync.Mutex
}

func NewSingleton(supplier Supplier) *Singleton {
	return &Singleton{
		supplier: supplier,
		cache:    make(map[string]any),
		locker:   &sync.Mutex{},
	}
}

func (s *Singleton) Get(key string) any {
	if value, hit := s.cache[key]; hit {
		return value
	}
	s.locker.Lock()
	defer s.locker.Unlock()
	if value, hit := s.cache[key]; hit {
		return value
	}
	s.cache[key] = s.supplier(key)
	return s.cache[key]
}
