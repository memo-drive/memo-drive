import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { MessageContainer } from "./components/base";
import { AuthGuard } from "./components/auth/AuthGuard";
import { MainLayout } from "./layouts/MainLayout";
import { DrivePage } from "./pages/Drive";
import { LoginPage } from "./pages/Login";
import { SmartSearchPage } from "./pages/SmartSearch";
import { SettingsPage } from "./pages/Settings";
import { TransferPage } from "./pages/Transfer";
import { TrashPage } from "./pages/Trash";

export default function App() {
  return (
    <BrowserRouter>
      <MessageContainer position="top-right" />
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          element={
            <AuthGuard>
              <MainLayout />
            </AuthGuard>
          }
        >
          <Route index element={<DrivePage />} />
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
