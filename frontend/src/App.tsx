import { Route, Routes } from 'react-router'
import { RequireAdmin, RequireAuth } from './auth/RequireAuth'
import { Layout } from './components/Layout'
import { AppointmentsPage } from './pages/AppointmentsPage'
import { CustomersPage } from './pages/CustomersPage'
import { DashboardPage } from './pages/DashboardPage'
import { LoginPage } from './pages/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { ServicesPage } from './pages/ServicesPage'
import { UsersPage } from './pages/UsersPage'

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<DashboardPage />} />
        <Route path="agenda" element={<AppointmentsPage />} />
        <Route path="servicos" element={<ServicesPage />} />
        <Route path="clientes" element={<CustomersPage />} />
        <Route
          path="usuarios"
          element={
            <RequireAdmin>
              <UsersPage />
            </RequireAdmin>
          }
        />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
