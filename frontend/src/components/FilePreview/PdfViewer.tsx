import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { Document, Page } from "react-pdf";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";
import { downloadUrl } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { PreviewShell } from "./PreviewShell";
import { PreviewToolbar } from "./PreviewToolbar";
import { PreviewError, PreviewLoading, PreviewUnsupported } from "./PreviewState";
import "./pdfWorker";
import styles from "./FilePreview.module.css";

const PDF_MAX_BYTES = 100 * 1024 * 1024;
const SCALE_OPTIONS = [
  { label: "50%", value: "0.5" },
  { label: "75%", value: "0.75" },
  { label: "100%", value: "1" },
  { label: "125%", value: "1.25" },
  { label: "150%", value: "1.5" },
  { label: "Fit Width", value: "fit" },
] as const;

type LoadedPDF = {
  numPages: number;
  destroy?: () => Promise<void> | void;
};

function withRetryParam(src: string, retryKey: number) {
  const sep = src.includes("?") ? "&" : "?";
  return `${src}${sep}preview_retry=${retryKey}`;
}

function pdfErrorMessage(message: string | undefined, t: (key: string) => string): string {
  const text = message?.toLowerCase() ?? "";
  if (text.includes("network") || text.includes("fetch")) {
    return t("preview.networkError");
  }
  if (text.includes("password") || text.includes("encrypted")) {
    return t("preview.pdfEncrypted");
  }
  return t("preview.pdfCorrupted");
}

export function PdfViewer({ file }: { file: DriveFile }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [pageInput, setPageInput] = useState("1");
  const [numPages, setNumPages] = useState<number | null>(null);
  const [scaleValue, setScaleValue] = useState<(typeof SCALE_OPTIONS)[number]["value"]>(
    "1",
  );
  const [bodyWidth, setBodyWidth] = useState(0);
  const [error, setError] = useState("");
  const [retryKey, setRetryKey] = useState(0);
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const pdfRef = useRef<LoadedPDF | null>(null);

  useEffect(() => {
    bodyRef.current?.focus();
  }, [file.id]);

  useEffect(() => {
    const node = bodyRef.current;
    if (!node) return;
    const updateWidth = () => setBodyWidth(Math.max(240, node.clientWidth - 32));
    updateWidth();
    const observer = new ResizeObserver(updateWidth);
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    return () => {
      void pdfRef.current?.destroy?.();
      pdfRef.current = null;
    };
  }, [file.id]);

  useEffect(() => {
    setPageInput(String(page));
  }, [page]);

  const source = useMemo(() => withRetryParam(downloadUrl(file.id), retryKey), [
    file.id,
    retryKey,
  ]);

  if (file.size > PDF_MAX_BYTES) {
    return <PreviewUnsupported file={file} reason={t("preview.fileTooLarge")} />;
  }

  const goToPage = (next: number) => {
    if (!numPages) {
      setPage(Math.max(1, next));
      return;
    }
    setPage(Math.min(numPages, Math.max(1, next)));
  };

  function applyPageInput() {
    const next = Number.parseInt(pageInput, 10);
    if (Number.isFinite(next)) {
      goToPage(next);
    } else {
      setPageInput(String(page));
    }
  }

  function retry() {
    setError("");
    setPage(1);
    setPageInput("1");
    setNumPages(null);
    setScaleValue("fit");
    setRetryKey((value) => value + 1);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowLeft" || event.key === "PageUp") {
      event.preventDefault();
      goToPage(page - 1);
    }
    if (event.key === "ArrowRight" || event.key === "PageDown") {
      event.preventDefault();
      goToPage(page + 1);
    }
    if (event.key === "Home") {
      event.preventDefault();
      goToPage(1);
    }
    if (event.key === "End" && numPages) {
      event.preventDefault();
      goToPage(numPages);
    }
  }

  const fixedScale = scaleValue === "fit" ? undefined : Number(scaleValue);

  return (
    <PreviewShell
      mode="fullbleed"
      bodyClassName={styles.pdfBody}
      toolbar={
        <PreviewToolbar>
          <button
            className={styles.toolButton}
            type="button"
            disabled={page <= 1}
            onClick={() => goToPage(page - 1)}
          >
            {t("preview.prevPage")}
          </button>
          <input
            className={styles.pageInput}
            value={pageInput}
            inputMode="numeric"
            aria-label={t("preview.pdfPageLabel")}
            onChange={(event) => setPageInput(event.target.value)}
            onBlur={applyPageInput}
            onKeyDown={(event) => {
              if (event.key === "Enter") applyPageInput();
            }}
          />
          <span className={styles.pageIndicator}>/ {numPages ?? "-"}</span>
          <button
            className={styles.toolButton}
            type="button"
            disabled={Boolean(numPages && page >= numPages)}
            onClick={() => goToPage(page + 1)}
          >
            {t("preview.nextPage")}
          </button>
          <select
            className={styles.select}
            value={scaleValue}
            onChange={(event) =>
              setScaleValue(
                event.target.value as (typeof SCALE_OPTIONS)[number]["value"],
              )
            }
            aria-label={t("preview.pdfZoomLabel")}
          >
            {SCALE_OPTIONS.map((item) => (
              <option key={item.value} value={item.value}>
                {item.label}
              </option>
            ))}
          </select>
        </PreviewToolbar>
      }
    >
      <div
        ref={bodyRef}
        tabIndex={0}
        className={styles.pdfCanvasWrap}
        onKeyDown={handleKeyDown}
      >
        {error ? (
          <PreviewError message={error} onRetry={retry} />
        ) : (
          <Document
            key={retryKey}
            file={source}
            loading={<PreviewLoading label={t("preview.loadingPdfText")} />}
            error={<PreviewError message={t("preview.pdfLoadFailed")} onRetry={retry} />}
            onLoadSuccess={(pdf: LoadedPDF) => {
              pdfRef.current = pdf;
              setNumPages(pdf.numPages);
              setPage(1);
              setError("");
            }}
            onLoadError={(err) => setError(pdfErrorMessage(err.message, t))}
          >
            <Page
              pageNumber={page}
              width={scaleValue === "fit" ? bodyWidth : undefined}
              scale={fixedScale}
              renderAnnotationLayer
              renderTextLayer
              loading={<PreviewLoading label={t("preview.renderingPage")} />}
            />
          </Document>
        )}
      </div>
    </PreviewShell>
  );
}
