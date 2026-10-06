package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
)

// Catalog is the PostgreSQL catalog.Repository.
type Catalog struct{ pool *pgxpool.Pool }

// NewCatalog returns a repository on pool.
func NewCatalog(pool *pgxpool.Pool) *Catalog { return &Catalog{pool} }

const itemCols = `id, name, description, duration_min, price_cents, active, created_at, updated_at`

type scanner interface{ Scan(dest ...any) error }

func scanItem(r scanner) (catalog.Item, error) {
	var it catalog.Item
	err := r.Scan(&it.ID, &it.Name, &it.Description, &it.DurationMin, &it.PriceCents, &it.Active, &it.CreatedAt, &it.UpdatedAt)
	it.CreatedAt, it.UpdatedAt = it.CreatedAt.UTC(), it.UpdatedAt.UTC()
	return it, err
}

func (s *Catalog) Create(ctx context.Context, it catalog.Item) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO services (`+itemCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		it.ID, it.Name, it.Description, it.DurationMin, it.PriceCents, it.Active, it.CreatedAt, it.UpdatedAt)
	return err
}

func (s *Catalog) Get(ctx context.Context, id string) (catalog.Item, error) {
	it, err := scanItem(s.pool.QueryRow(ctx, `SELECT `+itemCols+` FROM services WHERE id = $1`, id))
	if isNoRows(err) {
		return catalog.Item{}, catalog.ErrNotFound
	}
	return it, err
}

func (s *Catalog) List(ctx context.Context, f catalog.Filter) ([]catalog.Item, int, error) {
	// strpos (not ILIKE): the search text is literal, "%" and "_" need no escaping.
	const where = ` WHERE ($1::boolean IS NULL OR active = $1) AND ($2 = '' OR strpos(lower(name), lower($2)) > 0)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM services`+where, f.Active, f.Query).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+itemCols+` FROM services`+where+` ORDER BY name, id LIMIT $3 OFFSET $4`,
		f.Active, f.Query, f.PageSize, (f.Page-1)*f.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []catalog.Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

func (s *Catalog) Update(ctx context.Context, it catalog.Item) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE services SET name=$2, description=$3, duration_min=$4, price_cents=$5, active=$6, updated_at=$7 WHERE id=$1`,
		it.ID, it.Name, it.Description, it.DurationMin, it.PriceCents, it.Active, it.UpdatedAt)
	if err == nil && tag.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return err
}

func (s *Catalog) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM services WHERE id = $1`, id)
	if _, ok := violation(err, foreignKeyViolationCode); ok {
		return catalog.ErrInUse
	}
	if err == nil && tag.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return err
}
