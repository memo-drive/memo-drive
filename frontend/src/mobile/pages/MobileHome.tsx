import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, useLocation } from "react-router-dom";
import {
  listRecentlyViewedFiles,
  queryFiles,
} from "../../api/fileApi";
import { message } from "../../components/base";
import { useChunkedUpload } from "../../hooks/useChunkedUpload";
import { isActiveTransferStatus } from "../../stores/transferProjection";
import { useTransferStore } from "../../stores/transferStore";
import type { DriveFile } from "../../types";
import { MobileHomeView } from "./MobileHomeView";
import {
  buildMobileHomeSearchRequest,
  startMobileHomeUploads,
} from "./mobileHomeActions";

export function MobileHomePage() {
  const { t } = useTranslation();
  const location = useLocation();
  const tasks = useTransferStore((state) => state.tasks);
  const loadSessions = useTransferStore((state) => state.loadSessions);
  const [recentFiles, setRecentFiles] = useState<DriveFile[]>([]);
  const [recentLoading, setRecentLoading] = useState(false);
  const [searchDraft, setSearchDraft] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<DriveFile[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const transferCount = tasks.filter((task) => isActiveTransferStatus(task.status)).length;
  const { upload } = useChunkedUpload(() => undefined);
  const legacyPath = new URLSearchParams(location.search).get("path");

  useEffect(() => {
    void loadSessions();
  }, [loadSessions]);

  useEffect(() => {
    let cancelled = false;
    setRecentLoading(true);
    listRecentlyViewedFiles(10)
      .then((response) => {
        if (cancelled) return;
        setRecentFiles(response.files ?? []);
      })
      .catch(() => {
        if (cancelled) return;
        setRecentFiles([]);
      })
      .finally(() => {
        if (!cancelled) setRecentLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function submitSearch() {
    const request = buildMobileHomeSearchRequest(searchDraft);
    if (!request) return;
    setSearchQuery(request.query ?? "");
    setSearching(true);
    setSearchError("");
    try {
      const response = await queryFiles(request);
      setSearchResults(response.items ?? []);
    } catch (err) {
      setSearchResults([]);
      setSearchError(err instanceof Error ? err.message : t("drive.searchFailed"));
    } finally {
      setSearching(false);
    }
  }

  function updateSearchDraft(value: string) {
    setSearchDraft(value);
    if (!value.trim()) {
      clearSearch();
    }
  }

  function clearSearch() {
    setSearchDraft("");
    setSearchQuery("");
    setSearchResults([]);
    setSearchError("");
    setSearching(false);
  }

  function handleUploadFiles(selected: FileList | null) {
    const count = startMobileHomeUploads(selected, upload, (file, err) => {
      if (err instanceof Error && err.message === "upload cancelled") return;
      message.error(t("drive.uploadFailed", { name: file.name }));
    });
    if (count > 0) {
      message.info(t("drive.filesAddedToTransfer", { count }));
    }
  }

  return legacyPath ? (
    <Navigate to={`/m/files${location.search}`} replace />
  ) : (
    <MobileHomeView
      searchDraft={searchDraft}
      searchActive={Boolean(searchQuery)}
      searchResults={searchResults}
      searching={searching}
      searchError={searchError}
      transferCount={transferCount}
      recentFiles={recentFiles}
      recentLoading={recentLoading}
      onSearchDraftChange={updateSearchDraft}
      onSearchSubmit={() => void submitSearch()}
      onClearSearch={clearSearch}
      onUploadFiles={handleUploadFiles}
    />
  );
}
