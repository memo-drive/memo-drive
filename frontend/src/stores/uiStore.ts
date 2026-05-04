import { create } from "zustand";

interface UIState {
  previewOpen: boolean;
  setPreviewOpen: (open: boolean) => void;
}

export const useUIStore = create<UIState>((set) => ({
  previewOpen: true,
  setPreviewOpen: (previewOpen) => set({ previewOpen }),
}));
