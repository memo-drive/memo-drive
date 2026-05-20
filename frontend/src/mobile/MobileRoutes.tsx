import { Route, Routes } from "react-router-dom";
import { MobileShell } from "./layouts/MobileShell";
import { MobileAIPage } from "./pages/MobileAI";
import { MobileDrivePage } from "./pages/MobileDrive";
import { MobileHomePage } from "./pages/MobileHome";
import { MobileCategoryPage } from "./pages/MobileCategory";
import { MobileMediaPreviewPage } from "./pages/MobileMediaPreview";
import { MobileMePage } from "./pages/MobileMe";
import { MobilePreviewPage } from "./pages/MobilePreview";
import { MobileTransferPage } from "./pages/MobileTransfer";
import { MobileTrashPage } from "./pages/MobileTrash";
import "./styles/tokens.css";

export function MobileRoutes() {
  return (
    <Routes>
      <Route path="preview/:fileId" element={<MobilePreviewPage />} />
      <Route path="media/:category/:fileId" element={<MobileMediaPreviewPage />} />
      <Route path="category/:category" element={<MobileCategoryPage />} />
      <Route path="ai" element={<MobileAIPage />} />
      <Route element={<MobileShell />}>
        <Route index element={<MobileHomePage />} />
        <Route path="files" element={<MobileDrivePage />} />
        <Route path="transfer" element={<MobileTransferPage />} />
        <Route path="me" element={<MobileMePage />} />
        <Route path="trash" element={<MobileTrashPage />} />
      </Route>
    </Routes>
  );
}
