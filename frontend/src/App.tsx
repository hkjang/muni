import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useAuth } from "./contexts/AuthContext";
import { LoadingScreen } from "./components/LoadingScreen";
import { AppShell } from "./layouts/AppShell";
import { AdminShell } from "./layouts/AdminShell";
import { LoginPage } from "./pages/LoginPage";
import { DashboardPage } from "./pages/DashboardPage";
import { WorkspacePage } from "./pages/WorkspacePage";
import { SearchPage } from "./pages/SearchPage";
import { ApprovalsPage } from "./pages/ApprovalsPage";
import { PersonalSettingsPage } from "./pages/PersonalSettingsPage";
import { EditorPage } from "./pages/EditorPage";
import { AdminSettingsPage } from "./pages/admin/AdminSettingsPage";
import { AdminUsersPage } from "./pages/admin/AdminUsersPage";
import { AdminKeyPoliciesPage } from "./pages/admin/AdminKeyPoliciesPage";
import { AdminAuditPage } from "./pages/admin/AdminAuditPage";
import { AdminAIUsagePage } from "./pages/admin/AdminAIUsagePage";
import { NotFoundPage } from "./pages/NotFoundPage";

function Protected({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  const location = useLocation();
  if (loading) return <LoadingScreen />;
  if (!user)
    return (
      <Navigate
        to="/login"
        replace
        state={{ returnTo: location.pathname + location.search }}
      />
    );
  return children;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/docs/:documentId"
        element={
          <Protected>
            <EditorPage />
          </Protected>
        }
      />
      <Route
        element={
          <Protected>
            <AppShell />
          </Protected>
        }
      >
        <Route index element={<DashboardPage />} />
        <Route path="workspace/:workspaceId" element={<WorkspacePage />} />
        <Route path="favorites" element={<DashboardPage scope="favorites" />} />
        <Route path="shared" element={<DashboardPage scope="shared" />} />
        <Route path="trash" element={<DashboardPage scope="trash" />} />
        <Route path="search" element={<SearchPage />} />
        <Route path="approvals" element={<ApprovalsPage />} />
        <Route path="settings" element={<PersonalSettingsPage />} />
      </Route>
      <Route
        path="/admin"
        element={
          <Protected>
            <AdminShell />
          </Protected>
        }
      >
        <Route index element={<AdminSettingsPage />} />
        <Route path="users" element={<AdminUsersPage />} />
        <Route path="key-policies" element={<AdminKeyPoliciesPage />} />
        <Route path="ai-usage" element={<AdminAIUsagePage />} />
        <Route path="audit" element={<AdminAuditPage />} />
      </Route>
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
