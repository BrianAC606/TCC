package internel

import (
	"TCC/model"
	"errors"
	"fmt"
	"sync"
)

type RegistryCenter struct {
	mux        sync.RWMutex
	components map[string]model.TCCComponent
}

func NewRegistryCenter() *RegistryCenter {
	return &RegistryCenter{
		components: make(map[string]model.TCCComponent),
	}
}

func (rc *RegistryCenter) Register(component model.TCCComponent) error {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	if _, ok := rc.components[component.ID()]; ok {
		return errors.New("component already exists")
	}
	rc.components[component.ID()] = component
	return nil
}

func (rc *RegistryCenter) GetComponentByIDs(componentIDs ...string) ([]model.TCCComponent, error) {
	components := make([]model.TCCComponent, 0, len(componentIDs))
	for _, id := range componentIDs {
		if component, ok := rc.components[id]; ok {
			components = append(components, component)
		} else {
			return nil, fmt.Errorf("component id:%v does not exist", id)
		}
	}
	return components, nil
}
