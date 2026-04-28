package discovery

import (
	"context"
	"fmt"
	rand "math/rand/v2"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/discovery/consul"
	"github.com/sirupsen/logrus"
)

// Config holds everything the discovery package needs. All values are provided
// explicitly by the caller; no global configuration is read here.
type Config struct {
	ConsulAddr string // e.g. "localhost:8500"
	GRPCAddr   string // this service's own gRPC address, used for self-registration
}

// RegistryToConsul registers the service in Consul and starts a background
// health-check goroutine. The returned function must be called on shutdown to
// stop the goroutine and deregister the instance.
func RegistryToConsul(ctx context.Context, serviceName string, cfg Config) (func() error, error) {
	registry, err := consul.New(cfg.ConsulAddr)
	if err != nil {
		return func() error { return nil }, err
	}
	instanceID := generateInstanceID(serviceName)
	if err := registry.Register(ctx, instanceID, serviceName, cfg.GRPCAddr); err != nil {
		return func() error { return nil }, err
	}

	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if err := registry.HealthCheck(instanceID, serviceName); err != nil {
					logrus.WithFields(logrus.Fields{
						"serviceName": serviceName,
						"instanceID":  instanceID,
					}).WithError(err).Error("consul health check failed")
				}
			}
		}
	}()

	logrus.WithFields(logrus.Fields{
		"serviceName": serviceName,
		"addr":        cfg.GRPCAddr,
	}).Info("Registry to consul success")

	return func() error {
		close(stopCh)
		return registry.DeRegister(context.Background(), instanceID, serviceName)
	}, nil
}

// GetServiceAddr discovers a random live instance of serviceName from Consul.
func GetServiceAddr(ctx context.Context, serviceName string, consulAddr string) (string, error) {
	registry, err := consul.New(consulAddr)
	if err != nil {
		return "", err
	}
	addrs, err := registry.Discover(ctx, serviceName)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("got empty %s addrs from consul", serviceName)
	}
	i := rand.IntN(len(addrs))
	logrus.Infof("Discovered %d instances of %s, addrs=%v", len(addrs), serviceName, addrs)
	return addrs[i], nil
}
