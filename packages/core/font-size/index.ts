/**
 * Font-size preference store.
 *
 * Stores an integer percentage value (80 | 90 | 100 | 110 | 120) that maps to
 * the `--font-zoom` CSS custom property applied on the root element.
 * Defaults to 100 (no zoom). Persisted via defaultStorage (SSR-safe localStorage).
 */
import { createStore } from "zustand/vanilla";
import { persist, createJSONStorage } from "zustand/middleware";
import { useStore } from "zustand";
import { defaultStorage } from "../platform/storage";

export type FontSizeOption = 80 | 90 | 100 | 110 | 120;

export const FONT_SIZE_OPTIONS: FontSizeOption[] = [80, 90, 100, 110, 120];
export const FONT_SIZE_DEFAULT: FontSizeOption = 100;

export const FONT_SIZE_LABELS: Record<FontSizeOption, string> = {
  80: "XS",
  90: "S",
  100: "M",
  110: "L",
  120: "XL",
};

interface FontSizeState {
  fontSize: FontSizeOption;
  setFontSize: (size: FontSizeOption) => void;
}

export const fontSizeStore = createStore<FontSizeState>()(
  persist(
    (set) => ({
      fontSize: FONT_SIZE_DEFAULT,
      setFontSize: (fontSize) => set({ fontSize }),
    }),
    {
      name: "multica-font-size",
      storage: createJSONStorage(() => defaultStorage),
    },
  ),
);

export function useFontSizeStore(): FontSizeState;
export function useFontSizeStore<T>(selector: (state: FontSizeState) => T): T;
export function useFontSizeStore<T>(selector?: (state: FontSizeState) => T) {
  return useStore(fontSizeStore, selector as (state: FontSizeState) => T);
}
