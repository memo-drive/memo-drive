import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type {
  DriveFile,
  FileQueryDocumentSubtype,
  FileQueryMediaFilter,
  PhotoMonthIndexItem,
} from "../../types";
import { filePresentation, fileSizeLabel } from "../../components/FileManager/filePresentation";
import { LazyThumbnail, VirtualGrid, VirtualList } from "../../components/Virtualized";
import { MobileBatchActionBar, MobileSelectionTopBar } from "../selection/MobileSelectionBars";
import { useMobileLongPress } from "../selection/useMobileLongPress";
import {
  categoryFileHref,
  formatCategoryMonthLabel,
  type MobileCategoryKey,
} from "./mobileCategoryActions";
import styles from "./MobileCategory.module.css";

export type PhotoCategoryMode = "timeline" | "list";
export type VideoCategoryMode = "recent" | "all";

interface MobileCategoryViewProps {
  category: MobileCategoryKey;
  searchOpen?: boolean;
  searchDraft?: string;
  searchActive?: boolean;
  loading?: boolean;
  error?: string;
  files?: DriveFile[];
  hasMore?: boolean;
  isLoadingMore?: boolean;
  photoMode?: PhotoCategoryMode;
  videoMode?: VideoCategoryMode;
  videoFilter?: FileQueryMediaFilter | string;
  documentSubtype?: FileQueryDocumentSubtype | string;
  audioSort?: string;
  recentFiles?: DriveFile[];
  photoMonths?: PhotoMonthIndexItem[];
  activePhotoMonth?: PhotoMonthIndexItem | null;
  timelineFiles?: DriveFile[];
  timelineHasMore?: boolean;
  timelineLoading?: boolean;
  selectionActive?: boolean;
  selectedIds?: string[];
  selectedCount?: number;
  allSelected?: boolean;
  onOpenSearch?: () => void;
  onCancelSearch?: () => void;
  onClearSearch?: () => void;
  onSearchDraftChange?: (value: string) => void;
  onSearchSubmit?: () => void;
  onPhotoModeChange?: (mode: PhotoCategoryMode) => void;
  onVideoModeChange?: (mode: VideoCategoryMode) => void;
  onVideoFilterChange?: (filter: FileQueryMediaFilter | string) => void;
  onDocumentSubtypeChange?: (subtype: FileQueryDocumentSubtype | string) => void;
  onAudioSortChange?: (sort: string) => void;
  onPhotoMonthSelect?: (month: PhotoMonthIndexItem) => void;
  onListEndReached?: () => void;
  onTimelineEndReached?: () => void;
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
  onCancelSelection?: () => void;
  onSelectAll?: () => void;
  onBatchMove?: () => void;
  onBatchDelete?: () => void;
}

const CATEGORY_TITLE_KEYS: Record<MobileCategoryKey, string> = {
  photos: "mobile.category.photos.title",
  videos: "mobile.category.videos.title",
  documents: "mobile.category.documents.title",
  audio: "mobile.category.audio.title",
};

const CATEGORY_SEARCH_KEYS: Record<MobileCategoryKey, string> = {
  photos: "mobile.category.photos.searchPlaceholder",
  videos: "mobile.category.videos.searchPlaceholder",
  documents: "mobile.category.documents.searchPlaceholder",
  audio: "mobile.category.audio.searchPlaceholder",
};

const VIDEO_FILTERS: Array<{ key: FileQueryMediaFilter; labelKey: string }> = [
  { key: "all", labelKey: "mobile.category.filters.all" },
  { key: "lt_1m", labelKey: "mobile.category.videos.lt1m" },
  { key: "1_10m", labelKey: "mobile.category.videos.oneToTen" },
  { key: "gt_10m", labelKey: "mobile.category.videos.gt10m" },
];

const DOCUMENT_SUBTYPES: Array<{ key: FileQueryDocumentSubtype; labelKey: string }> = [
  { key: "all", labelKey: "mobile.category.filters.all" },
  { key: "pdf", labelKey: "mobile.category.documents.pdf" },
  { key: "text", labelKey: "mobile.category.documents.text" },
  { key: "spreadsheet", labelKey: "mobile.category.documents.spreadsheet" },
  { key: "presentation", labelKey: "mobile.category.documents.presentation" },
  { key: "txt", labelKey: "mobile.category.documents.txt" },
  { key: "other", labelKey: "mobile.category.documents.other" },
];

const AUDIO_SORTS = [
  { key: "updated_at", labelKey: "mobile.category.sort.updatedAt" },
  { key: "name", labelKey: "mobile.category.sort.name" },
  { key: "size", labelKey: "mobile.category.sort.size" },
];

export function MobileCategoryView({
  category,
  searchOpen = false,
  searchDraft = "",
  searchActive = false,
  loading = false,
  error = "",
  files = [],
  hasMore = false,
  isLoadingMore = false,
  photoMode = "timeline",
  videoMode = "all",
  videoFilter = "all",
  documentSubtype = "all",
  audioSort = "updated_at",
  recentFiles = [],
  photoMonths = [],
  activePhotoMonth = null,
  timelineFiles = [],
  timelineHasMore = false,
  timelineLoading = false,
  selectionActive = false,
  selectedIds = [],
  selectedCount = selectedIds.length,
  allSelected = false,
  onOpenSearch,
  onCancelSearch,
  onClearSearch,
  onSearchDraftChange,
  onSearchSubmit,
  onPhotoModeChange,
  onVideoModeChange,
  onVideoFilterChange,
  onDocumentSubtypeChange,
  onAudioSortChange,
  onPhotoMonthSelect,
  onListEndReached,
  onTimelineEndReached,
  onLongPressFile,
  onToggleSelection,
  onCancelSelection,
  onSelectAll,
  onBatchMove,
  onBatchDelete,
}: MobileCategoryViewProps) {
  const { t } = useTranslation();
  const hasModeTabbar = category === "photos" || category === "videos";

  return (
    <section
      className={`${styles.page} ${hasModeTabbar ? "" : styles.pageNoModeTabbar}`}
      data-mobile-page={`category-${category}`}
    >
      {selectionActive ? (
        <MobileSelectionTopBar
          selectedCount={selectedCount}
          allSelected={allSelected}
          onCancel={() => onCancelSelection?.()}
          onSelectAll={() => onSelectAll?.()}
        />
      ) : (
        <CategoryHeader
          title={t(CATEGORY_TITLE_KEYS[category])}
          placeholder={t(CATEGORY_SEARCH_KEYS[category])}
          searchOpen={searchOpen}
          searchDraft={searchDraft}
          searchActive={searchActive}
          onOpenSearch={onOpenSearch}
          onCancelSearch={onCancelSearch}
          onClearSearch={onClearSearch}
          onSearchDraftChange={onSearchDraftChange}
          onSearchSubmit={onSearchSubmit}
        />
      )}
      <main className={styles.content}>
        {category === "photos" ? (
          photoMode === "timeline" ? (
            <PhotoTimeline
              months={photoMonths}
              activeMonth={activePhotoMonth}
              files={timelineFiles}
              loading={timelineLoading}
              error={error}
              hasMore={timelineHasMore}
              onMonthSelect={onPhotoMonthSelect}
              onEndReached={onTimelineEndReached}
              selectionActive={selectionActive}
              selectedIds={selectedIds}
              onLongPressFile={onLongPressFile}
              onToggleSelection={onToggleSelection}
            />
          ) : (
            <CategoryVirtualList
              category={category}
              files={files}
              loading={loading}
              error={error}
              hasMore={hasMore}
              isLoadingMore={isLoadingMore}
              onEndReached={onListEndReached}
              selectionActive={selectionActive}
              selectedIds={selectedIds}
              onLongPressFile={onLongPressFile}
              onToggleSelection={onToggleSelection}
            />
          )
        ) : category === "videos" ? (
          <VideosContent
            files={files}
            recentFiles={recentFiles}
            videoFilter={videoFilter}
            loading={loading}
            error={error}
            hasMore={hasMore}
            isLoadingMore={isLoadingMore}
            onVideoFilterChange={onVideoFilterChange}
            onEndReached={onListEndReached}
            selectionActive={selectionActive}
            selectedIds={selectedIds}
            onLongPressFile={onLongPressFile}
            onToggleSelection={onToggleSelection}
          />
        ) : category === "documents" ? (
          <DocumentsContent
            files={files}
            documentSubtype={documentSubtype}
            loading={loading}
            error={error}
            hasMore={hasMore}
            isLoadingMore={isLoadingMore}
            onDocumentSubtypeChange={onDocumentSubtypeChange}
            onEndReached={onListEndReached}
            selectionActive={selectionActive}
            selectedIds={selectedIds}
            onLongPressFile={onLongPressFile}
            onToggleSelection={onToggleSelection}
          />
        ) : (
          <AudioContent
            files={files}
            audioSort={audioSort}
            loading={loading}
            error={error}
            hasMore={hasMore}
            isLoadingMore={isLoadingMore}
            onAudioSortChange={onAudioSortChange}
            onEndReached={onListEndReached}
            selectionActive={selectionActive}
            selectedIds={selectedIds}
            onLongPressFile={onLongPressFile}
            onToggleSelection={onToggleSelection}
          />
        )}
      </main>
      {selectionActive ? (
        <MobileBatchActionBar
          selectedCount={selectedCount}
          onMove={() => onBatchMove?.()}
          onDelete={() => onBatchDelete?.()}
        />
      ) : (
        hasModeTabbar ? (
          <CategoryModeTabbar
            category={category}
            photoMode={photoMode}
            videoMode={videoMode}
            onPhotoModeChange={onPhotoModeChange}
            onVideoModeChange={onVideoModeChange}
          />
        ) : null
      )}
    </section>
  );
}

function CategoryHeader({
  title,
  placeholder,
  searchOpen,
  searchDraft,
  searchActive,
  onOpenSearch,
  onCancelSearch,
  onClearSearch,
  onSearchDraftChange,
  onSearchSubmit,
}: {
  title: string;
  placeholder: string;
  searchOpen: boolean;
  searchDraft: string;
  searchActive: boolean;
  onOpenSearch?: () => void;
  onCancelSearch?: () => void;
  onClearSearch?: () => void;
  onSearchDraftChange?: (value: string) => void;
  onSearchSubmit?: () => void;
}) {
  const { t } = useTranslation();
  const canClear = searchActive || searchDraft.trim().length > 0;

  if (searchOpen) {
    return (
      <form
        className={styles.searchForm}
        role="search"
        onSubmit={(event) => {
          event.preventDefault();
          onSearchSubmit?.();
        }}
      >
        <span className="material-symbols-outlined" aria-hidden>
          search
        </span>
        <input
          value={searchDraft}
          placeholder={placeholder}
          onChange={(event) => onSearchDraftChange?.(event.target.value)}
        />
        {canClear ? (
          <button
            className={styles.searchClearButton}
            type="button"
            aria-label={t("searchResultList.clearSearch")}
            onClick={onClearSearch}
          >
            <span className="material-symbols-outlined" aria-hidden>
              close
            </span>
          </button>
        ) : (
          <button className={styles.searchCancelButton} type="button" onClick={onCancelSearch}>
            {t("common.cancel")}
          </button>
        )}
      </form>
    );
  }

  return (
    <header className={styles.header}>
      <button type="button" className={styles.iconButton} aria-label={t("common.search")} onClick={onOpenSearch}>
        <span className="material-symbols-outlined" aria-hidden>
          search
        </span>
      </button>
      <h1>{title}</h1>
      <Link className={styles.iconButton} to="/m" aria-label={t("mobile.nav.home")}>
        <span className="material-symbols-outlined" aria-hidden>
          home
        </span>
      </Link>
    </header>
  );
}

function PhotoTimeline({
  months,
  activeMonth,
  files,
  loading,
  error,
  hasMore,
  onMonthSelect,
  onEndReached,
  selectionActive,
  selectedIds,
  onLongPressFile,
  onToggleSelection,
}: {
  months: PhotoMonthIndexItem[];
  activeMonth: PhotoMonthIndexItem | null;
  files: DriveFile[];
  loading: boolean;
  error: string;
  hasMore: boolean;
  onMonthSelect?: (month: PhotoMonthIndexItem) => void;
  onEndReached?: () => void;
  selectionActive: boolean;
  selectedIds: string[];
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
}) {
  const { t } = useTranslation();

  if (loading && files.length === 0 && months.length === 0) {
    return <StateBlock>{t("common.loading")}</StateBlock>;
  }
  if (error) {
    return <StateBlock>{error}</StateBlock>;
  }
  if (!activeMonth) {
    return <StateBlock>{t("mobile.category.photos.empty")}</StateBlock>;
  }

  return (
    <section className={styles.timeline}>
      <div className={styles.timelineHeader}>
        <h2>{formatCategoryMonthLabel(activeMonth)}</h2>
        <small>{t("mobile.category.photos.count", { count: activeMonth.count })}</small>
      </div>
      <VirtualGrid
        className={styles.photoGrid}
        itemClassName={styles.photoCell}
        items={files}
        height="calc(100dvh - 12.5rem)"
        estimateRowHeight={88}
        minColumnWidth={78}
        gap={2}
        maxColumns={5}
        hasMore={hasMore}
        isLoading={loading}
        getItemKey={(file) => file.id}
        renderItem={(file) => (
          <PhotoTile
            file={file}
            selectionActive={selectionActive}
            selected={selectedIds.includes(file.id)}
            onLongPressFile={onLongPressFile}
            onToggleSelection={onToggleSelection}
          />
        )}
        onEndReached={onEndReached}
      />
      <nav className={styles.monthRail} aria-label={t("mobile.category.photos.monthQuickNav")}>
        {months.map((month) => {
          const label = formatCategoryMonthLabel(month);
          const active = month.year === activeMonth.year && month.month === activeMonth.month;
          return (
            <button
              key={`${month.year}-${month.month}`}
              className={active ? styles.monthRailActive : ""}
              type="button"
              aria-label={t("mobile.category.photos.jumpToMonth", { month: label })}
              onClick={() => onMonthSelect?.(month)}
            >
              <span>{month.month}</span>
            </button>
          );
        })}
      </nav>
    </section>
  );
}

function PhotoTile({
  file,
  selectionActive,
  selected,
  onLongPressFile,
  onToggleSelection,
}: {
  file: DriveFile;
  selectionActive: boolean;
  selected: boolean;
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
}) {
  const longPress = useMobileLongPress(onLongPressFile ? () => onLongPressFile(file) : undefined);
  return (
    <Link
      className={`${styles.photoTile} ${selectionActive ? styles.categorySelectable : ""}`}
      to={selectionActive ? "#" : categoryFileHref("photos", file)}
      data-mobile-category-selectable={selectionActive || undefined}
      data-mobile-photo-selected={selectionActive ? String(selected) : undefined}
      aria-selected={selectionActive ? selected : undefined}
      onPointerDown={longPress.onPointerDown}
      onPointerUp={longPress.onPointerUp}
      onPointerLeave={longPress.onPointerLeave}
      onPointerCancel={longPress.onPointerCancel}
      onContextMenu={longPress.onContextMenu}
      onClick={(event) => {
        if (longPress.consumeClickAfterLongPress()) {
          event.preventDefault();
          return;
        }
        if (selectionActive) {
          event.preventDefault();
          onToggleSelection?.(file);
        }
      }}
    >
      <LazyThumbnail
        file={file}
        className={styles.photoThumb}
        placeholderClassName={styles.photoPlaceholder}
        fallback={
          <span className="material-symbols-outlined" aria-hidden>
            image
          </span>
        }
      />
      <span className={styles.photoName}>{file.name}</span>
      {selectionActive ? (
        <em className={styles.categorySelectionMark} aria-hidden>
          {selected ? <span className="material-symbols-outlined">check</span> : null}
        </em>
      ) : null}
    </Link>
  );
}

function VideosContent({
  files,
  recentFiles,
  videoFilter,
  loading,
  error,
  hasMore,
  isLoadingMore,
  onVideoFilterChange,
  onEndReached,
  selectionActive,
  selectedIds,
  onLongPressFile,
  onToggleSelection,
}: {
  files: DriveFile[];
  recentFiles: DriveFile[];
  videoFilter: FileQueryMediaFilter | string;
  loading: boolean;
  error: string;
  hasMore: boolean;
  isLoadingMore: boolean;
  onVideoFilterChange?: (filter: FileQueryMediaFilter | string) => void;
  onEndReached?: () => void;
  selectionActive: boolean;
  selectedIds: string[];
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <section className={styles.recentSection}>
        <header>
          <h2>{t("mobile.category.videos.recent")}</h2>
        </header>
        {recentFiles.length === 0 ? (
          <p>{t("mobile.category.videos.recentEmpty")}</p>
        ) : (
          <div className={styles.recentRail}>
            {recentFiles.map((file) => (
              <Link key={file.id} to={categoryFileHref("videos", file)}>
                <LazyThumbnail
                  file={file}
                  className={styles.recentThumb}
                  placeholderClassName={styles.recentPlaceholder}
                  fallback={<span className="material-symbols-outlined" aria-hidden>movie</span>}
                />
                <strong>{file.name}</strong>
              </Link>
            ))}
          </div>
        )}
      </section>
      <ChipBar
        items={VIDEO_FILTERS}
        activeKey={videoFilter}
        onSelect={onVideoFilterChange}
      />
      <CategoryVirtualList
        category="videos"
        files={files}
        loading={loading}
        error={error}
        hasMore={hasMore}
        isLoadingMore={isLoadingMore}
        onEndReached={onEndReached}
        selectionActive={selectionActive}
        selectedIds={selectedIds}
        onLongPressFile={onLongPressFile}
        onToggleSelection={onToggleSelection}
      />
    </>
  );
}

function DocumentsContent({
  files,
  documentSubtype,
  loading,
  error,
  hasMore,
  isLoadingMore,
  onDocumentSubtypeChange,
  onEndReached,
  selectionActive,
  selectedIds,
  onLongPressFile,
  onToggleSelection,
}: {
  files: DriveFile[];
  documentSubtype: FileQueryDocumentSubtype | string;
  loading: boolean;
  error: string;
  hasMore: boolean;
  isLoadingMore: boolean;
  onDocumentSubtypeChange?: (subtype: FileQueryDocumentSubtype | string) => void;
  onEndReached?: () => void;
  selectionActive: boolean;
  selectedIds: string[];
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
}) {
  return (
    <>
      <ChipBar
        items={DOCUMENT_SUBTYPES}
        activeKey={documentSubtype}
        onSelect={onDocumentSubtypeChange}
      />
      <CategoryVirtualList
        category="documents"
        files={files}
        loading={loading}
        error={error}
        hasMore={hasMore}
        isLoadingMore={isLoadingMore}
        onEndReached={onEndReached}
        selectionActive={selectionActive}
        selectedIds={selectedIds}
        onLongPressFile={onLongPressFile}
        onToggleSelection={onToggleSelection}
      />
    </>
  );
}

function AudioContent({
  files,
  audioSort,
  loading,
  error,
  hasMore,
  isLoadingMore,
  onAudioSortChange,
  onEndReached,
  selectionActive,
  selectedIds,
  onLongPressFile,
  onToggleSelection,
}: {
  files: DriveFile[];
  audioSort: string;
  loading: boolean;
  error: string;
  hasMore: boolean;
  isLoadingMore: boolean;
  onAudioSortChange?: (sort: string) => void;
  onEndReached?: () => void;
  selectionActive: boolean;
  selectedIds: string[];
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
}) {
  return (
    <>
      <ChipBar
        items={AUDIO_SORTS}
        activeKey={audioSort}
        onSelect={onAudioSortChange}
      />
      <CategoryVirtualList
        category="audio"
        files={files}
        loading={loading}
        error={error}
        hasMore={hasMore}
        isLoadingMore={isLoadingMore}
        onEndReached={onEndReached}
        selectionActive={selectionActive}
        selectedIds={selectedIds}
        onLongPressFile={onLongPressFile}
        onToggleSelection={onToggleSelection}
      />
    </>
  );
}

function CategoryVirtualList({
  category,
  files,
  loading,
  error,
  hasMore,
  isLoadingMore,
  onEndReached,
  selectionActive,
  selectedIds,
  onLongPressFile,
  onToggleSelection,
}: {
  category: MobileCategoryKey;
  files: DriveFile[];
  loading: boolean;
  error: string;
  hasMore: boolean;
  isLoadingMore: boolean;
  onEndReached?: () => void;
  selectionActive: boolean;
  selectedIds: string[];
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
}) {
  const { t } = useTranslation();

  if (loading && files.length === 0) {
    return <StateBlock>{t("common.loading")}</StateBlock>;
  }
  if (error) {
    return <StateBlock>{error}</StateBlock>;
  }
  if (files.length === 0) {
    return <StateBlock>{t("searchResultList.noResults")}</StateBlock>;
  }

  return (
    <VirtualList
      className={styles.virtualList}
      itemClassName={styles.virtualRow}
      items={files}
      height="calc(100dvh - 13.5rem)"
      estimateSize={82}
      hasMore={hasMore}
      isLoading={isLoadingMore}
      getItemKey={(file) => file.id}
      renderItem={(file) => (
        <CategoryFileRow
          category={category}
          file={file}
          selectionActive={selectionActive}
          selected={selectedIds.includes(file.id)}
          onLongPressFile={onLongPressFile}
          onToggleSelection={onToggleSelection}
        />
      )}
      onEndReached={onEndReached}
    />
  );
}

function CategoryFileRow({
  category,
  file,
  selectionActive,
  selected,
  onLongPressFile,
  onToggleSelection,
}: {
  category: MobileCategoryKey;
  file: DriveFile;
  selectionActive: boolean;
  selected: boolean;
  onLongPressFile?: (file: DriveFile) => void;
  onToggleSelection?: (file: DriveFile) => void;
}) {
  const presentation = filePresentation(file);
  const icon = (
    <span className="material-symbols-outlined" aria-hidden>
      {presentation.iconName}
    </span>
  );
  const longPress = useMobileLongPress(onLongPressFile ? () => onLongPressFile(file) : undefined);
  const showThumbnail = presentation.kind === "image" || presentation.kind === "video";

  return (
    <Link
      className={`${styles.fileRow} ${selectionActive ? styles.categorySelectable : ""} ${selected ? styles.categorySelected : ""}`}
      to={selectionActive ? "#" : categoryFileHref(category, file)}
      data-mobile-category-selectable={selectionActive || undefined}
      aria-selected={selectionActive ? selected : undefined}
      onPointerDown={longPress.onPointerDown}
      onPointerUp={longPress.onPointerUp}
      onPointerLeave={longPress.onPointerLeave}
      onPointerCancel={longPress.onPointerCancel}
      onContextMenu={longPress.onContextMenu}
      onClick={(event) => {
        if (longPress.consumeClickAfterLongPress()) {
          event.preventDefault();
          return;
        }
        if (selectionActive) {
          event.preventDefault();
          onToggleSelection?.(file);
        }
      }}
    >
      <span className={styles.fileIcon}>
        {showThumbnail ? (
          <LazyThumbnail
            file={file}
            className={styles.rowThumb}
            placeholderClassName={styles.rowPlaceholder}
            fallback={icon}
          />
        ) : (
          icon
        )}
      </span>
      <span className={styles.fileText}>
        <strong>{file.name}</strong>
        <small>
          {dateLabel(file.updated_at)} · {fileSizeLabel(file)}
        </small>
      </span>
      {selectionActive ? (
        <span className={styles.categorySelectionMark} aria-hidden>
          <span className="material-symbols-outlined">
            {selected ? "check_circle" : "radio_button_unchecked"}
          </span>
        </span>
      ) : (
        <span className="material-symbols-outlined" aria-hidden>
          chevron_right
        </span>
      )}
    </Link>
  );
}

function ChipBar<T extends string>({
  items,
  activeKey,
  onSelect,
}: {
  items: Array<{ key: T; labelKey: string }>;
  activeKey: T | string;
  onSelect?: (key: T) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className={styles.chipBar}>
      {items.map((item) => (
        <button
          key={item.key}
          className={activeKey === item.key ? styles.chipActive : ""}
          type="button"
          aria-pressed={activeKey === item.key}
          onClick={() => onSelect?.(item.key)}
        >
          {t(item.labelKey)}
        </button>
      ))}
    </div>
  );
}

function CategoryModeTabbar({
  category,
  photoMode,
  videoMode,
  onPhotoModeChange,
  onVideoModeChange,
}: {
  category: MobileCategoryKey;
  photoMode: PhotoCategoryMode;
  videoMode: VideoCategoryMode;
  onPhotoModeChange?: (mode: PhotoCategoryMode) => void;
  onVideoModeChange?: (mode: VideoCategoryMode) => void;
}) {
  const { t } = useTranslation();
  const items =
    category === "photos"
      ? [
          { key: "timeline", icon: "image", labelKey: "mobile.category.photos.timeline", active: photoMode === "timeline" },
          { key: "list", icon: "view_list", labelKey: "mobile.category.photos.list", active: photoMode === "list" },
        ]
      : category === "videos"
        ? [
            { key: "recent", icon: "history", labelKey: "mobile.category.videos.recent", active: videoMode === "recent" },
            { key: "all", icon: "video_library", labelKey: "mobile.category.videos.all", active: videoMode === "all" },
          ]
        : category === "documents"
          ? [{ key: "all", icon: "description", labelKey: "mobile.category.documents.all", active: true }]
          : [{ key: "all", icon: "headphones", labelKey: "mobile.category.audio.all", active: true }];

  return (
    <nav className={styles.modeTabbar} aria-label={t("mobile.category.modeNav")}>
      {items.map((item) => (
        <button
          key={item.key}
          className={item.active ? styles.modeTabActive : ""}
          type="button"
          aria-pressed={item.active}
          onClick={() => {
            if (category === "photos") onPhotoModeChange?.(item.key as PhotoCategoryMode);
            if (category === "videos") onVideoModeChange?.(item.key as VideoCategoryMode);
          }}
        >
          <span className="material-symbols-outlined" aria-hidden>
            {item.icon}
          </span>
          <span>{t(item.labelKey)}</span>
        </button>
      ))}
    </nav>
  );
}

function StateBlock({ children }: { children: React.ReactNode }) {
  return <div className={styles.state}>{children}</div>;
}

function dateLabel(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toISOString().slice(0, 10);
}
