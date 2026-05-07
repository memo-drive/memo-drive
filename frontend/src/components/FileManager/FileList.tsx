import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { DriveFile } from "../../types";
import { Popover } from "../base";
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

	function getIconClass(file: DriveFile) {
		if (file.is_dir) return styles.iconBoxDir;
		if (file.mime_type.startsWith("image/")) return styles.iconBoxImg;
		return styles.iconBoxFile;
	}

	function getIconName(file: DriveFile) {
		if (file.is_dir) return "folder";
		if (file.mime_type.startsWith("image/")) return "image";
		if (file.mime_type.startsWith("video/")) return "video_library";
		if (file.mime_type.startsWith("audio/")) return "audio_file";
		return "description";
	}

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
						<tr
							key={file.id}
							className={`${styles.tableRow} ${selectedId === file.id ? styles.tableRowSelected : ""}`}
							onClick={() => {
								onSelect(file);
								if (file.is_dir) onOpenFolder(file);
							}}
						>
							<td className={styles.tableCell}>
								<div className={styles.fileIconWrapper}>
									<div
										className={`${styles.iconBox} ${getIconClass(file)}`}
									>
										<span className="material-symbols-outlined">
											{getIconName(file)}
										</span>
									</div>
									<div>
										<p className={styles.fileName}>
											{file.name}
										</p>
										<p className={styles.fileDesc}>
											{file.is_dir
												? "Folder"
												: file.mime_type || "File"}
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
									{file.is_dir ? "--" : formatSize(file.size)}
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

function formatSize(size: number) {
	if (size < 1024) return `${size} B`;
	if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
	if (size < 1024 * 1024 * 1024)
		return `${(size / 1024 / 1024).toFixed(1)} MB`;
	return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}
