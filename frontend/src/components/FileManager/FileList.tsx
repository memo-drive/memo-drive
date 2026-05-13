import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { thumbnailUrl } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { Popover } from "../base";
import {
	filePresentation,
	fileSizeLabel,
	type FilePresentationKind,
} from "./filePresentation";
import styles from "./FileList.module.css";

interface Props {
	files: DriveFile[];
	selectedId?: string;
	onOpenFolder: (file: DriveFile) => void;
	onSelect: (file: DriveFile) => void;
	onDelete: (file: DriveFile) => void;
	onRename: (file: DriveFile) => void;
	onMove: (file: DriveFile) => void;
	onDownload: (file: DriveFile) => void;
}

const PAGE_SIZE = 50;

export function FileList({
	files,
	selectedId,
	onOpenFolder,
	onSelect,
	onDelete,
	onRename,
	onMove,
	onDownload,
}: Props) {
	const { t } = useTranslation();
	const [page, setPage] = useState(1);

	useEffect(() => {
		setPage(1);
	}, [files]);


	if (!files || files.length === 0) {
		return (
			<div className="p-8 text-center text-zinc-500">
				{t("fileList.empty")}
			</div>
		);
	}

	const totalPages = Math.ceil(files?.length / PAGE_SIZE);
	const paged = files.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

	const iconClasses: Record<FilePresentationKind, string> = {
		audio: styles.iconBoxFile,
		file: styles.iconBoxFile,
		folder: styles.iconBoxDir,
		image: styles.iconBoxImg,
		video: styles.iconBoxFile,
	};

	return (
		<div className={styles.tableWrapper}>
			<table className={styles.table}>
				<thead>
					<tr className={styles.tableHeadRow}>
						<th className={styles.tableHeadCell}>{t("fileList.name")}</th>
						<th className={styles.tableHeadCell}>{t("fileList.modified")}</th>
						<th className={styles.tableHeadCell}>{t("fileList.size")}</th>
						<th className={styles.tableHeadCellRight}>{t("fileList.actions")}</th>
					</tr>
				</thead>
				<tbody className={styles.tbody}>
					{paged.map((file) => (
						<FileRow
							key={file.id}
							file={file}
							iconClasses={iconClasses}
							onDelete={onDelete}
							onDownload={onDownload}
							onMove={onMove}
							onOpenFolder={onOpenFolder}
							onRename={onRename}
							onSelect={onSelect}
							selected={selectedId === file.id}
							t={t}
						/>
					))}
				</tbody>
			</table>
			{totalPages > 1 && (
				<div className={styles.pagination}>
					<button
						className={styles.pageBtn}
						disabled={page <= 1}
						onClick={() => setPage((p) => p - 1)}
					>
						<span className="material-symbols-outlined">
							chevron_left
						</span>
					</button>
					<span className={styles.pageInfo}>
						{t("fileList.pagination", { current: page, total: totalPages, count: files.length })}
					</span>
					<button
						className={styles.pageBtn}
						disabled={page >= totalPages}
						onClick={() => setPage((p) => p + 1)}
					>
						<span className="material-symbols-outlined">
							chevron_right
						</span>
					</button>
				</div>
			)}
		</div>
	);
}

interface FileRowProps {
	file: DriveFile;
	iconClasses: Record<FilePresentationKind, string>;
	onOpenFolder: (file: DriveFile) => void;
	onSelect: (file: DriveFile) => void;
	onDelete: (file: DriveFile) => void;
	onRename: (file: DriveFile) => void;
	onMove: (file: DriveFile) => void;
	onDownload: (file: DriveFile) => void;
	selected: boolean;
	t: (key: string, options?: Record<string, unknown>) => string;
}

function FileRow({
	file,
	iconClasses,
	onDelete,
	onDownload,
	onMove,
	onOpenFolder,
	onRename,
	onSelect,
	selected,
	t,
}: FileRowProps) {
	const presentation = filePresentation(file);
	return (
		<tr
			className={`${styles.tableRow} ${selected ? styles.tableRowSelected : ""}`}
			onClick={() => {
				onSelect(file);
				if (file.is_dir) onOpenFolder(file);
			}}
		>
			<td className={styles.tableCell}>
				<div className={styles.fileIconWrapper}>
					<div
						className={`${styles.iconBox} ${iconClasses[presentation.kind]}`}
					>
						<FileIcon file={file} presentation={presentation} />
					</div>
					<div>
						<p className={styles.fileName}>
							{file.name}
						</p>
						<p className={styles.fileDesc}>
							{presentation.description}
						</p>
					</div>
				</div>
			</td>
							<td className={styles.tableCell}>
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
							</td>
							<td className={styles.tableCell}>
								<span className={styles.metaText}>
									{fileSizeLabel(file)}
								</span>
							</td>
							<td className={styles.tableCellRight}>
								<Popover
									placement="bottom-end"
									content={
										<div className={styles.popoverMenu}>
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
										onClick={(e: any) =>
											e.stopPropagation()
										}
									>
										<span className="material-symbols-outlined">
											more_vert
										</span>
									</button>
								</Popover>
							</td>
						</tr>
	);
}

function FileIcon({
	file,
	presentation,
}: {
	file: DriveFile;
	presentation: ReturnType<typeof filePresentation>;
}) {
	const [thumbnailFailed, setThumbnailFailed] = useState(false);
	const hasThumbnail =
		file.status === "ready" &&
		Boolean(file.metadata?.thumbnail_path) &&
		(presentation.kind === "image" || presentation.kind === "video");
	if (hasThumbnail && !thumbnailFailed) {
		return (
			<img
				className={styles.thumbnail}
				src={thumbnailUrl(file.id)}
				alt={file.name}
				loading="lazy"
				decoding="async"
				onError={() => setThumbnailFailed(true)}
			/>
		);
	}

	return (
		<span className="material-symbols-outlined">
			{presentation.iconName}
		</span>
	);
}
