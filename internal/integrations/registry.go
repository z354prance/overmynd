package integrations

import (
	"fmt"
	"sync"

	"github.com/z354prance/overmynd/internal/models"
)

type Registry struct {
	mu           sync.RWMutex
	integrations map[models.ServiceType]Integration
}

func NewRegistry() *Registry {
	return &Registry{
		integrations: make(
			map[models.ServiceType]Integration,
		),
	}
}

func (r *Registry) Register(integration Integration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	serviceType := integration.Type()

	if _, exists := r.integrations[serviceType]; exists {
		return fmt.Errorf(
			"integration already registered: %s",
			serviceType,
		)
	}

	r.integrations[serviceType] = integration

	return nil
}

func (r *Registry) Get(
	serviceType models.ServiceType,
) (Integration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	integration, ok := r.integrations[serviceType]

	return integration, ok
}

func (r *Registry) Types() []models.ServiceType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]models.ServiceType, 0, len(r.integrations))

	for serviceType := range r.integrations {
		types = append(types, serviceType)
	}

	return types
}
