import { useEffect } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { MessageContainer } from "./components/base";
import { AuthGuard } from "./components/auth/AuthGuard";
import { MainLayout } from "./layouts/MainLayout";
import { currentUserAgent, mobileRootEntryForUserAgent } from "./mobile/mobileEntry";
import { MobileRoutes } from "./mobile/MobileRoutes";
import { DrivePage } from "./pages/Drive";
import { LoginPage } from "./pages/Login";
import { SmartSearchPage } from "./pages/SmartSearch";
import { SettingsPage } from "./pages/Settings";
import { TransferPage } from "./pages/Transfer";
import { TrashPage } from "./pages/Trash";

export default function App() {
  const { i18n } = useTranslation();

  useEffect(() => {
    document.documentElement.lang = i18n.language || "zh-CN";
  }, [i18n.language]);

  return (
    <BrowserRouter>
      <MessageContainer position="top-right" />
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          path="/m/*"
          element={
            <AuthGuard>
              <MobileRoutes />
            </AuthGuard>
          }
        />
        <Route
          element={
            <AuthGuard>
              <MainLayout />
            </AuthGuard>
          }
        >
          <Route index element={<DesktopRootEntry />} />
          <Route path="smart-search" element={<SmartSearchPage />} />
          <Route path="transfer" element={<TransferPage />} />
          <Route path="trash" element={<TrashPage />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

function DesktopRootEntry() {
  const mobileEntry = mobileRootEntryForUserAgent("/", currentUserAgent());
  if (mobileEntry) {
    return <Navigate to={mobileEntry} replace />;
  }

  return <DrivePage />;
}
