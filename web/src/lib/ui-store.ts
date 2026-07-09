"use client";

import { create } from "zustand";

// Ephemeral UI state shared across the shell: the command palette, the
// shortcuts dialog, mobile navigation and the currently requested action.
// Ring-card buttons and command-palette items both funnel into pendingAction;
// the dashboard renders the matching (confirmation) dialog.

export type PendingAction =
  | { type: "seed"; ring?: string }
  | { type: "promote"; fromRing: string }
  | { type: "rollback"; ring: string };

interface UiState {
  paletteOpen: boolean;
  shortcutsOpen: boolean;
  mobileNavOpen: boolean;
  pendingAction: PendingAction | null;
  setPaletteOpen: (open: boolean) => void;
  setShortcutsOpen: (open: boolean) => void;
  setMobileNavOpen: (open: boolean) => void;
  setPendingAction: (action: PendingAction | null) => void;
}

export const useUiStore = create<UiState>()((set) => ({
  paletteOpen: false,
  shortcutsOpen: false,
  mobileNavOpen: false,
  pendingAction: null,
  setPaletteOpen: (paletteOpen) => set({ paletteOpen }),
  setShortcutsOpen: (shortcutsOpen) => set({ shortcutsOpen }),
  setMobileNavOpen: (mobileNavOpen) => set({ mobileNavOpen }),
  setPendingAction: (pendingAction) => set({ pendingAction }),
}));
