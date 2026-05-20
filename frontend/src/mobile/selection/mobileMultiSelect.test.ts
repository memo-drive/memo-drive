import { describe, expect, it } from "vitest";
import {
  cancelMobileMultiSelect,
  createMobileMultiSelectState,
  enterMobileMultiSelect,
  isMobileMultiSelectAllSelected,
  isMobileMultiSelectSelected,
  resetMobileMultiSelectForContext,
  selectAllMobileMultiSelectItems,
  toggleMobileMultiSelectItem,
} from "./mobileMultiSelect";

describe("mobile multi-select state", () => {
  it("enters selection in the current context, toggles items, and cancels back to browsing", () => {
    let state = createMobileMultiSelectState("home:recent");

    state = enterMobileMultiSelect(state, "home:recent", "file-1");
    expect(state.active).toBe(true);
    expect(state.contextKey).toBe("home:recent");
    expect(state.selectedIds).toEqual(["file-1"]);
    expect(isMobileMultiSelectSelected(state, "file-1")).toBe(true);

    state = toggleMobileMultiSelectItem(state, "file-2");
    expect(state.selectedIds).toEqual(["file-1", "file-2"]);

    state = toggleMobileMultiSelectItem(state, "file-1");
    expect(state.active).toBe(true);
    expect(state.selectedIds).toEqual(["file-2"]);

    state = cancelMobileMultiSelect(state);
    expect(state.active).toBe(false);
    expect(state.selectedIds).toEqual([]);
    expect(state.contextKey).toBe("home:recent");
  });

  it("selects all visible items in order and reports whether the current context is fully selected", () => {
    let state = createMobileMultiSelectState("category:photos:list");

    state = selectAllMobileMultiSelectItems(state, "category:photos:list", [
      "photo-1",
      "photo-2",
      "photo-2",
      "photo-3",
    ]);

    expect(state.active).toBe(true);
    expect(state.selectedIds).toEqual(["photo-1", "photo-2", "photo-3"]);
    expect(isMobileMultiSelectAllSelected(state, ["photo-1", "photo-2", "photo-3"])).toBe(true);
    expect(isMobileMultiSelectAllSelected(state, ["photo-1", "photo-4"])).toBe(false);
  });

  it("clears selection when the caller switches to another browsing context", () => {
    let state = enterMobileMultiSelect(
      createMobileMultiSelectState("files:/Docs"),
      "files:/Docs",
      "file-1",
    );

    state = toggleMobileMultiSelectItem(state, "file-2");
    expect(state.selectedIds).toEqual(["file-1", "file-2"]);

    const sameContext = resetMobileMultiSelectForContext(state, "files:/Docs");
    expect(sameContext).toEqual(state);

    const nextContext = resetMobileMultiSelectForContext(state, "files:/Images");
    expect(nextContext.active).toBe(false);
    expect(nextContext.contextKey).toBe("files:/Images");
    expect(nextContext.selectedIds).toEqual([]);
  });
});
