import { useEffect, useState } from "react";
import { useLocation, useParams } from "react-router-dom";
import { downloadUrl, getFile } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { mobilePreviewReturnHref } from "../utils/mobilePath";
import { MobilePreviewView } from "./MobilePreviewView";

export function MobilePreviewPage() {
  const { fileId } = useParams();
  const location = useLocation();
  const returnHref = mobilePreviewReturnHref(location.search);
  const [file, setFile] = useState<DriveFile | undefined>();
  const [loading, setLoading] = useState(Boolean(fileId));
  const [error, setError] = useState("");

  useEffect(() => {
    if (!fileId) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    getFile(fileId)
      .then((nextFile) => {
        if (cancelled) return;
        setFile(nextFile);
        setError("");
      })
      .catch((err) => {
        if (cancelled) return;
        setFile(undefined);
        setError(err instanceof Error ? err.message : "Failed to load file");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [fileId]);

  return (
    <MobilePreviewView
      file={file}
      returnHref={returnHref}
      downloadHref={file ? downloadUrl(file.id) : undefined}
      loading={loading}
      error={error}
    />
  );
}
