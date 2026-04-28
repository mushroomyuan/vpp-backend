package discovery

import (
	"context"
	"fmt"
	rand "math/rand/v2"
)

type Registry interface {
	Register(ctx context.Context, instanceID, serviceName, hostPort string) error
	DeRegister(ctx context.Context, instanceID, serviceName string) error
	Discover(ctx context.Context, serviceName string) ([]string, error)
	HealthCheck(instanceID, serviceName string) error
}

func generateInstanceID(serviceName string) string {
	return fmt.Sprintf("%s-%d", serviceName, rand.Int())
}
