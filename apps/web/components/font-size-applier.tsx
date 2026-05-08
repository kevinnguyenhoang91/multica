"use client";

import { useEffect } from "react";
import { useFontSizeStore } from "@multica/core/font-size";

/**
 * Reads the persisted font-size preference and applies it as a CSS custom
 * property `--font-zoom` on the root element. This lets any part of the UI
 * scale text by referencing `calc(1rem * var(--font-zoom, 1))`.
 *
 * Rendered once near the top of the tree (inside ThemeProvider / WebProviders)
 * so the value is applied before visible content renders.
 */
export function FontSizeApplier() {
  const fontSize = useFontSizeStore((s) => s.fontSize);

  useEffect(() => {
    document.documentElement.style.setProperty(
      "--font-zoom",
      String(fontSize / 100),
    );
  }, [fontSize]);

  return null;
}
