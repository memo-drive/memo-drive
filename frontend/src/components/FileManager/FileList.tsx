import { useTranslation } from "react-i18next";
import type { DriveFile } from "../../types";
import { Popover } from "../base";
import { LazyThumbnail, VirtualList } from "../Virtualized";
import { isMarkdownFile } from "../FilePreview/textTypes";
import {
	filePresentation,
	fileSizeLabel,
	type FilePresentationKind,
} from "./filePresentation";
import styles from "./FileList.module.css";

interface Props {
	files: DriveFile[];
	selectedId?: string;
	enteringFolderId?: string | null;
	folderNavigationDisabled?: boolean;
	onOpenFolder: (file: DriveFile) => void;
	onSelect: (file: DriveFile) => void;
	onDelete: (file: DriveFile) => void;
	onEdit?: (file: DriveFile) => void;
	onRename: (file: DriveFile) => void;
	onMove: (file: DriveFile) => void;
	onDownload: (file: DriveFile) => void;
}

export function canEditMarkdownFile(file: DriveFile): boolean {
	return !file.is_dir && isMarkdownFile(file);
}

export function FileList({
	files,
	selectedId,
	enteringFolderId = null,
	folderNavigationDisabled = false,
	onOpenFolder,
	onSelect,
	onDelete,
	onEdit,
	onRename,
	onMove,
	onDownload,
}: Props) {
	const { t } = useTranslation();

	if (!files || files.length === 0) {
		return (
			<div className="p-8 text-center text-zinc-500">
				{t("fileList.empty")}
			</div>
		);
	}

	const iconClasses: Record<FilePresentationKind, string> = {
		audio: styles.iconBoxFile,
		file: styles.iconBoxFile,
		folder: styles.iconBoxDir,
		image: styles.iconBoxImg,
		video: styles.iconBoxFile,
	};
	const busy = folderNavigationDisabled || Boolean(enteringFolderId);

	return (
		<div
			className={styles.tableWrapper}
			aria-busy={busy || undefined}
			data-file-list-busy={busy || undefined}
		>
			<div className={styles.table} role="table" aria-disabled={busy || undefined}>
				<div className={styles.tableHeadRow} role="row">
					<div className={styles.tableHeadCell} role="columnheader">{t("fileList.name")}</div>
					<div className={styles.tableHeadCell} role="columnheader">{t("fileList.modified")}</div>
					<div className={styles.tableHeadCell} role="columnheader">{t("fileList.size")}</div>
					<div className={styles.tableHeadCellRight} role="columnheader">{t("fileList.actions")}</div>
				</div>
				<VirtualList
					className={styles.virtualBody}
					itemClassName={styles.virtualSlot}
					items={files}
					height="calc(100% - 3.125rem)"
					estimateSize={73}
					overscan={8}
					role="rowgroup"
					getItemKey={(file) => file.id}
					renderItem={(file) => (
						<FileRow
							key={file.id}
							file={file}
							iconClasses={iconClasses}
							entering={enteringFolderId === file.id}
							fileListBusy={busy}
							onDelete={onDelete}
							onDownload={onDownload}
							onEdit={onEdit}
							onMove={onMove}
							onOpenFolder={onOpenFolder}
							onRename={onRename}
							onSelect={onSelect}
							selected={selectedId === file.id}
							t={t}
						/>
					)}
				/>
			</div>
			{busy ? (
				<div className={styles.busyOverlay} role="status" aria-live="polite">
					<LoadingSpinnerIcon className={styles.loadingIcon} />
					<span>{t("fileList.loadingFolder")}</span>
				</div>
			) : null}
		</div>
	);
}

interface FileRowProps {
	file: DriveFile;
	iconClasses: Record<FilePresentationKind, string>;
	entering: boolean;
	fileListBusy: boolean;
	onOpenFolder: (file: DriveFile) => void;
	onSelect: (file: DriveFile) => void;
	onDelete: (file: DriveFile) => void;
	onEdit?: (file: DriveFile) => void;
	onRename: (file: DriveFile) => void;
	onMove: (file: DriveFile) => void;
	onDownload: (file: DriveFile) => void;
	selected: boolean;
	t: (key: string, options?: Record<string, unknown>) => string;
}

function FileRow({
	file,
	iconClasses,
	entering,
	fileListBusy,
	onDelete,
	onDownload,
	onEdit,
	onMove,
	onOpenFolder,
	onRename,
	onSelect,
	selected,
	t,
}: FileRowProps) {
	const presentation = filePresentation(file);
	return (
		<div
			className={`${styles.tableRow} ${selected ? styles.tableRowSelected : ""} ${fileListBusy ? styles.tableRowDisabled : ""}`}
			aria-busy={entering || undefined}
			aria-disabled={fileListBusy || undefined}
			onClick={() => {
				if (fileListBusy) return;
				onSelect(file);
				if (file.is_dir) onOpenFolder(file);
			}}
			role="row"
		>
			<div className={styles.tableCell} role="cell">
				<div className={styles.fileIconWrapper}>
					<div
						className={`${styles.iconBox} ${iconClasses[presentation.kind]}`}
					>
						{entering ? (
							<LoadingSpinnerIcon className={styles.loadingIcon} />
						) : (
							<FileIcon file={file} presentation={presentation} />
						)}
					</div>
					<div>
						<p className={styles.fileName}>
							{file.name}
						</p>
						<p className={styles.fileDesc}>
							{entering ? t("fileList.enteringFolder") : presentation.description}
						</p>
					</div>
				</div>
			</div>
			<div className={styles.tableCell} role="cell">
				<span className={styles.metaText}>
					{new Date(file.updated_at).toLocaleString(
						undefined,
						{
							year: "numeric",
							month: "short",
							day: "numeric",
							hour: "2-digit",
							minute: "2-digit",
						},
					)}
				</span>
			</div>
			<div className={styles.tableCell} role="cell">
				<span className={styles.metaText}>
					{fileSizeLabel(file)}
				</span>
			</div>
			<div className={styles.tableCellRight} role="cell">
				<Popover
					placement="bottom-end"
					content={
						<div className={styles.popoverMenu}>
							{canEditMarkdownFile(file) && onEdit ? (
								<button
									className={styles.menuItem}
									onClick={(e: any) => {
										e.stopPropagation();
										onEdit(file);
									}}
								>
									<span className="material-symbols-outlined text-[18px]">
										edit_note
									</span>{" "}
									{t("common.edit")}
								</button>
							) : null}
							<button
								className={styles.menuItem}
								onClick={(e: any) => {
									e.stopPropagation();
									onRename(file);
								}}
							>
								<span className="material-symbols-outlined text-[18px]">
									edit
								</span>{" "}
								{t("common.rename")}
							</button>
							<button
								className={styles.menuItem}
								onClick={(e: any) => {
									e.stopPropagation();
									onMove(file);
								}}
							>
								<span className="material-symbols-outlined text-[18px]">
									drive_file_move
								</span>{" "}
								{t("common.moveTo")}
							</button>
							{!file.is_dir && (
								<button
									className={styles.menuItem}
									onClick={(e: any) => {
										e.stopPropagation();
										onDownload(file);
									}}
								>
									<span className="material-symbols-outlined text-[18px]">
										download
									</span>{" "}
									{t("common.download")}
								</button>
							)}
							<button
								className={styles.menuItemDanger}
								onClick={(e: any) => {
									e.stopPropagation();
									onDelete(file);
								}}
							>
								<span className="material-symbols-outlined text-[18px]">
									delete
								</span>{" "}
								{t("drive.deleteToTrash")}
							</button>
						</div>
					}
				>
					<button
						className={styles.actionButton}
						disabled={fileListBusy}
						onClick={(e: any) =>
							e.stopPropagation()
						}
					>
						<span className="material-symbols-outlined">
							more_vert
						</span>
					</button>
				</Popover>
			</div>
		</div>
	);
}

function LoadingSpinnerIcon({ className = "" }: { className?: string }) {
	return (
		<svg
			className={className}
			viewBox="0 0 24 24"
			fill="none"
			aria-hidden="true"
			focusable="false"
		>
			<circle
				cx="12"
				cy="12"
				r="9"
				stroke="currentColor"
				strokeWidth="3"
				opacity="0.18"
			/>
			<path
				d="M21 12a9 9 0 0 0-9-9"
				stroke="currentColor"
				strokeLinecap="round"
				strokeWidth="3"
			/>
		</svg>
	);
}

function FileIcon({
	file,
	presentation,
}: {
	file: DriveFile;
	presentation: ReturnType<typeof filePresentation>;
}) {
	const hasThumbnail =
		file.status === "ready" &&
		Boolean(file.metadata?.thumbnail_path) &&
		(presentation.kind === "image" || presentation.kind === "video");
	const icon = (
		<span className="material-symbols-outlined">
			{presentation.iconName}
		</span>
	);
	if (hasThumbnail) {
		return (
			<LazyThumbnail
				file={file}
				className={styles.thumbnail}
				fallback={icon}
			/>
		);
	}

	return icon;
}
