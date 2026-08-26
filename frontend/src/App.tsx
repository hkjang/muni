import { lazy, Suspense } from "react";
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useParams,
} from "react-router-dom";
import { useAuth } from "./contexts/AuthContext";
import { LoadingScreen } from "./components/LoadingScreen";
import { AppShell } from "./layouts/AppShell";
import { LoginPage } from "./pages/LoginPage";
import { DashboardPage } from "./pages/DashboardPage";
import { WorkspacePage } from "./pages/WorkspacePage";
import { SearchPage } from "./pages/SearchPage";
import { ApprovalsPage } from "./pages/ApprovalsPage";
import { PersonalSettingsPage } from "./pages/PersonalSettingsPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { ChangeTemporaryPasswordPage } from "./pages/ChangeTemporaryPasswordPage";

// Everything used to arrive in one 1.6 MB file. The sign-in page downloaded
// the editor and every administration screen before it could show a password
// box, and — the part that actually matters — so did the public share page,
// which is served to people outside the organisation who have no account and
// no business receiving the administration interface at all.
//
// Split by who needs what. The editor carries ProseMirror and Yjs; the
// administration screens are read by a handful of people; the share page is
// read by strangers.
const EditorPage = lazy(() =>
  import("./pages/EditorPage").then((m) => ({ default: m.EditorPage })),
);
const SharedDocumentPage = lazy(() =>
  import("./pages/SharedDocumentPage").then((m) => ({
    default: m.SharedDocumentPage,
  })),
);
const AdminShell = lazy(() =>
  import("./layouts/AdminShell").then((m) => ({ default: m.AdminShell })),
);
const AdminOverviewPage = lazy(() =>
  import("./pages/admin/AdminOverviewPage").then((m) => ({
    default: m.AdminOverviewPage,
  })),
);
const AdminSettingsPage = lazy(() =>
  import("./pages/admin/AdminSettingsPage").then((m) => ({
    default: m.AdminSettingsPage,
  })),
);
const AdminWorkspacesPage = lazy(() =>
  import("./pages/admin/AdminWorkspacesPage").then((m) => ({
    default: m.AdminWorkspacesPage,
  })),
);
const AdminDocumentsPage = lazy(() =>
  import("./pages/admin/AdminDocumentsPage").then((m) => ({
    default: m.AdminDocumentsPage,
  })),
);
const AdminUsersPage = lazy(() =>
  import("./pages/admin/AdminUsersPage").then((m) => ({
    default: m.AdminUsersPage,
  })),
);
const AdminKeyPoliciesPage = lazy(() =>
  import("./pages/admin/AdminKeyPoliciesPage").then((m) => ({
    default: m.AdminKeyPoliciesPage,
  })),
);
const AdminAuditPage = lazy(() =>
  import("./pages/admin/AdminAuditPage").then((m) => ({
    default: m.AdminAuditPage,
  })),
);
const AdminAIUsagePage = lazy(() =>
  import("./pages/admin/AdminAIUsagePage").then((m) => ({
    default: m.AdminAIUsagePage,
  })),
);

function SharedRoute() {
  const { token } = useParams();
  return <SharedDocumentPage token={token ?? ""} />;
}

/** A split route is a network round trip; this is what fills it. */
function Loading({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={<LoadingScreen />}>{children}</Suspense>;
}

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
  // Every protected route funnels through here, so this is the one place the
  // check belongs. The server refuses the requests anyway; this turns that
  // refusal into a screen that says what to do about it.
  if (user.mustChangePassword) return <ChangeTemporaryPasswordPage />;
  return children;
}

export default function App() {
  return (
    <Loading>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        {/* Outside Protected on purpose: whoever opens this has no account. */}
        <Route path="/s/:token" element={<SharedRoute />} />
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
          <Route
            path="favorites"
            element={<DashboardPage scope="favorites" />}
          />
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
          <Route index element={<AdminOverviewPage />} />
          <Route path="settings" element={<AdminSettingsPage />} />
          <Route path="workspaces" element={<AdminWorkspacesPage />} />
          <Route path="documents" element={<AdminDocumentsPage />} />
          <Route path="users" element={<AdminUsersPage />} />
          <Route path="key-policies" element={<AdminKeyPoliciesPage />} />
          <Route path="ai-usage" element={<AdminAIUsagePage />} />
          <Route path="audit" element={<AdminAuditPage />} />
        </Route>
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Loading>
  );
}
