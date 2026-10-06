package memstore

import (
	"testing"

	"github.com/andreccls/go-react-business-suite/backend/internal/auth"
	"github.com/andreccls/go-react-business-suite/backend/internal/repotest"
)

func TestUsersContract(t *testing.T) {
	repotest.AuthStore(t, func(*testing.T) auth.Store { return NewUsers() })
}
