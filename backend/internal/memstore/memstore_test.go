package memstore

import (
	"testing"

	"github.com/andreccls/go-react-business-suite/backend/internal/repotest"
)

func factory(*testing.T) repotest.Stores {
	db := New()
	return repotest.Stores{Catalog: db.Catalog(), Customers: db.Customers(), Appointments: db.Appointments(), Dashboard: db.Dashboard()}
}

func TestCatalogContract(t *testing.T)      { repotest.Catalog(t, factory) }
func TestCustomersContract(t *testing.T)    { repotest.Customers(t, factory) }
func TestAppointmentsContract(t *testing.T) { repotest.Appointments(t, factory) }
func TestDashboardContract(t *testing.T)    { repotest.Dashboard(t, factory) }
