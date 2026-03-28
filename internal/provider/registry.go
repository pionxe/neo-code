package provider

import (
	"errors"
	"sort"
	"strings"
)

type Registry struct {
	items map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{items: map[string]Provider{}}
}

func (r *Registry) Register(provider Provider) {
	if provider == nil {
		return
	}
	r.items[strings.ToLower(provider.Name())] = provider
}

func (r *Registry) Get(name string) (Provider, error) {
	item, ok := r.items[strings.ToLower(name)]
	if !ok {
		return nil, errors.New("provider: not found")
	}
	return item, nil
}

func (r *Registry) Descriptors() []ProviderDescriptor {
	return r.filteredDescriptors(func(ProviderDescriptor) bool { return true })
}

func (r *Registry) MVPDescriptors() []ProviderDescriptor {
	return r.filteredDescriptors(func(desc ProviderDescriptor) bool {
		return desc.SupportLevel == SupportLevelMVP && desc.MVPVisible
	})
}

func (r *Registry) AvailableDescriptors() []ProviderDescriptor {
	return r.filteredDescriptors(func(desc ProviderDescriptor) bool {
		return desc.Available
	})
}

func (r *Registry) filteredDescriptors(keep func(ProviderDescriptor) bool) []ProviderDescriptor {
	if r == nil || len(r.items) == 0 {
		return nil
	}

	descriptors := make([]ProviderDescriptor, 0, len(r.items))
	for _, item := range r.items {
		desc := Describe(item)
		if keep != nil && !keep(desc) {
			continue
		}
		descriptors = append(descriptors, desc)
	}

	sort.Slice(descriptors, func(i, j int) bool {
		return strings.ToLower(descriptors[i].Name) < strings.ToLower(descriptors[j].Name)
	})

	return descriptors
}
