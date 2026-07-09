"use client";

import { create } from "zustand";

// Ephemeral UI state shared across the shell: the command palette, the
// shortcuts dialog, mobile navigation and the currently requested action.
// Ring-card buttons and command-palette items both funnel into pendingAction;
// the dashboard renders the matching (confirmation) dialog.

// Every pending action is tagged with the app it targets: the dialogs only
// honor an action for the app whose dashboard is showing, so an action queued
// from another view (e.g. the command palette while a group page was open)
// can never fire against the wrong app.
export type PendingAction =
  | { type: "seed"; app: string; ring?: string }
  | { type: "promote"; app: string; fromRing: string }
  | { type: "rollback"; app: string; ring: string }
  // Enabling auto-promote INTO production needs the prod password — this
  // routes the switch through a confirmation dialog that collects it.
  | { type: "autoPromote"; app: string; ring: string };

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
