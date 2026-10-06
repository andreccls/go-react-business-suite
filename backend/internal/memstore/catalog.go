package memstore

import (
	"context"

	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
)

// Catalog is an in-memory catalog.Repository.
type Catalog struct{ d *DB }

func (c *Catalog) Create(_ context.Context, it catalog.Item) error {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	c.d.items[it.ID] = it
	return nil
}

func (c *Catalog) Get(_ context.Context, id string) (catalog.Item, error) {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	it, ok := c.d.items[id]
	if !ok {
		return catalog.Item{}, catalog.ErrNotFound
	}
	return it, nil
}

func (c *Catalog) List(_ context.Context, f catalog.Filter) ([]catalog.Item, int, error) {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	var out []catalog.Item
	for _, it := range c.d.items {
		if (f.Active == nil || it.Active == *f.Active) && (f.Query == "" || contains(it.Name, f.Query)) {
			out = append(out, it)
		}
	}
	sortBy(out, func(a, b catalog.Item) bool {
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return page(out, f.Page, f.PageSize), len(out), nil
}

func (c *Catalog) Update(_ context.Context, it catalog.Item) error {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	if _, ok := c.d.items[it.ID]; !ok {
		return catalog.ErrNotFound
	}
	c.d.items[it.ID] = it
	return nil
}

func (c *Catalog) Delete(_ context.Context, id string) error {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	if _, ok := c.d.items[id]; !ok {
		return catalog.ErrNotFound
	}
	for _, a := range c.d.appointments {
		if a.ServiceID == id {
			return catalog.ErrInUse
		}
	}
	delete(c.d.items, id)
	return nil
}
