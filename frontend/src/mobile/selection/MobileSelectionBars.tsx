import { useTranslation } from "react-i18next";
import styles from "./MobileSelectionBars.module.css";

interface MobileSelectionTopBarProps {
  selectedCount: number;
  allSelected: boolean;
  onCancel: () => void;
  onSelectAll: () => void;
}

interface MobileBatchActionBarProps {
  selectedCount: number;
  busy?: boolean;
  onMove: () => void;
  onDelete: () => void;
}

export function MobileSelectionTopBar({
  selectedCount,
  allSelected,
  onCancel,
  onSelectAll,
}: MobileSelectionTopBarProps) {
  const { t } = useTranslation();

  return (
    <header className={styles.topBar} data-mobile-selection-top="true">
      <button type="button" onClick={onCancel}>
        {t("common.cancel")}
      </button>
      <strong>{t("mobile.selection.selectedCount", { count: selectedCount })}</strong>
      <button type="button" onClick={onSelectAll} aria-pressed={allSelected}>
        {allSelected ? t("mobile.selection.allSelected") : t("mobile.selection.selectAll")}
      </button>
    </header>
  );
}

export function MobileBatchActionBar({
  selectedCount,
  busy = false,
  onMove,
  onDelete,
}: MobileBatchActionBarProps) {
  const { t } = useTranslation();
  const disabled = selectedCount <= 0 || busy;

  return (
    <nav className={styles.batchBar} data-mobile-batch-bar="true" aria-label={t("mobile.selection.batchActions")}>
      <button type="button" disabled={disabled} onClick={onMove}>
        <span className="material-symbols-outlined" aria-hidden>
          drive_file_move
        </span>
        <span>{t("common.moveTo")}</span>
      </button>
      <button className={styles.danger} type="button" disabled={disabled} onClick={onDelete}>
        <span className="material-symbols-outlined" aria-hidden>
          delete
        </span>
        <span>{t("common.delete")}</span>
      </button>
    </nav>
  );
}
