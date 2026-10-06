package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
)

// Customers is the PostgreSQL customer.Repository.
type Customers struct{ pool *pgxpool.Pool }

// NewCustomers returns a repository on pool.
func NewCustomers(pool *pgxpool.Pool) *Customers { return &Customers{pool} }

const customerCols = `id, name, email, phone, notes, created_at, updated_at`

func scanCustomer(r scanner) (customer.Customer, error) {
	var c customer.Customer
	err := r.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	return c, err
}

func emailTaken(err error) bool {
	c, ok := violation(err, uniqueViolationCode)
	return ok && c == "customers_email_key"
}

func (s *Customers) Create(ctx context.Context, c customer.Customer) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO customers (`+customerCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.Name, c.Email, c.Phone, c.Notes, c.CreatedAt, c.UpdatedAt)
	if emailTaken(err) {
		return customer.ErrEmailTaken
	}
	return err
}

func (s *Customers) Get(ctx context.Context, id string) (customer.Customer, error) {
	c, err := scanCustomer(s.pool.QueryRow(ctx, `SELECT `+customerCols+` FROM customers WHERE id = $1`, id))
	if isNoRows(err) {
		return customer.Customer{}, customer.ErrNotFound
	}
	return c, err
}

func (s *Customers) List(ctx context.Context, f customer.Filter) ([]customer.Customer, int, error) {
	const where = ` WHERE $1 = '' OR strpos(lower(name), lower($1)) > 0 OR strpos(email, lower($1)) > 0 OR strpos(phone, $1) > 0`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM customers`+where, f.Query).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+customerCols+` FROM customers`+where+` ORDER BY name, id LIMIT $2 OFFSET $3`,
		f.Query, f.PageSize, (f.Page-1)*f.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []customer.Customer{}
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, c)
	}
	return items, total, rows.Err()
}

func (s *Customers) Update(ctx context.Context, c customer.Customer) error {
	tag, err := s.pool.Exec(ctx, `UPDATE customers SET name=$2, email=$3, phone=$4, notes=$5, updated_at=$6 WHERE id=$1`,
		c.ID, c.Name, c.Email, c.Phone, c.Notes, c.UpdatedAt)
	if emailTaken(err) {
		return customer.ErrEmailTaken
	}
	if err == nil && tag.RowsAffected() == 0 {
		return customer.ErrNotFound
	}
	return err
}

func (s *Customers) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM customers WHERE id = $1`, id)
	if _, ok := violation(err, foreignKeyViolationCode); ok {
		return customer.ErrInUse
	}
	if err == nil && tag.RowsAffected() == 0 {
		return customer.ErrNotFound
	}
	return err
}
