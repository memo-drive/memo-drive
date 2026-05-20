export interface MobileMultiSelectState {
  active: boolean;
  contextKey: string;
  selectedIds: string[];
}

export function createMobileMultiSelectState(contextKey = ""): MobileMultiSelectState {
  return {
    active: false,
    contextKey,
    selectedIds: [],
  };
}

export function enterMobileMultiSelect(
  state: MobileMultiSelectState,
  contextKey: string,
  itemId: string,
): MobileMultiSelectState {
  return {
    active: true,
    contextKey,
    selectedIds: uniqueIds([itemId]),
  };
}

export function toggleMobileMultiSelectItem(
  state: MobileMultiSelectState,
  itemId: string,
): MobileMultiSelectState {
  return {
    ...state,
    active: true,
    selectedIds: state.selectedIds.includes(itemId)
      ? state.selectedIds.filter((id) => id !== itemId)
      : uniqueIds([...state.selectedIds, itemId]),
  };
}

export function resetMobileMultiSelectForContext(
  state: MobileMultiSelectState,
  contextKey: string,
): MobileMultiSelectState {
  if (state.contextKey === contextKey) return state;
  return createMobileMultiSelectState(contextKey);
}

export function cancelMobileMultiSelect(
  state: MobileMultiSelectState,
): MobileMultiSelectState {
  return {
    ...state,
    active: false,
    selectedIds: [],
  };
}

export function selectAllMobileMultiSelectItems(
  state: MobileMultiSelectState,
  contextKey: string,
  itemIds: string[],
): MobileMultiSelectState {
  const selectedIds = uniqueIds(itemIds);
  return {
    ...state,
    active: selectedIds.length > 0,
    contextKey,
    selectedIds,
  };
}

export function isMobileMultiSelectSelected(
  state: MobileMultiSelectState,
  itemId: string,
): boolean {
  return state.selectedIds.includes(itemId);
}

export function isMobileMultiSelectAllSelected(
  state: MobileMultiSelectState,
  itemIds: string[],
): boolean {
  const visibleIds = uniqueIds(itemIds);
  return (
    state.active &&
    visibleIds.length > 0 &&
    visibleIds.every((id) => state.selectedIds.includes(id))
  );
}

function uniqueIds(ids: string[]): string[] {
  return [...new Set(ids.filter(Boolean))];
}
