package memstore

import (
	"context"

	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
)

// Customers is an in-memory customer.Repository.
type Customers struct{ d *DB }

func (c *Customers) emailTaken(email, exceptID string) bool {
	for _, o := range c.d.customers {
		if o.Email == email && o.ID != exceptID {
			return true
		}
	}
	return false
}

func (c *Customers) Create(_ context.Context, cu customer.Customer) error {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	if c.emailTaken(cu.Email, "") {
		return customer.ErrEmailTaken
	}
	c.d.customers[cu.ID] = cu
	return nil
}

func (c *Customers) Get(_ context.Context, id string) (customer.Customer, error) {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	cu, ok := c.d.customers[id]
	if !ok {
		return customer.Customer{}, customer.ErrNotFound
	}
	return cu, nil
}

func (c *Customers) List(_ context.Context, f customer.Filter) ([]customer.Customer, int, error) {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	var out []customer.Customer
	for _, cu := range c.d.customers {
		if f.Query == "" || contains(cu.Name, f.Query) || contains(cu.Email, f.Query) || contains(cu.Phone, f.Query) {
			out = append(out, cu)
		}
	}
	sortBy(out, func(a, b customer.Customer) bool {
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return page(out, f.Page, f.PageSize), len(out), nil
}

func (c *Customers) Update(_ context.Context, cu customer.Customer) error {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	if _, ok := c.d.customers[cu.ID]; !ok {
		return customer.ErrNotFound
	}
	if c.emailTaken(cu.Email, cu.ID) {
		return customer.ErrEmailTaken
	}
	c.d.customers[cu.ID] = cu
	return nil
}

func (c *Customers) Delete(_ context.Context, id string) error {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	if _, ok := c.d.customers[id]; !ok {
		return customer.ErrNotFound
	}
	for _, a := range c.d.appointments {
		if a.CustomerID == id {
			return customer.ErrInUse
		}
	}
	delete(c.d.customers, id)
	return nil
}
