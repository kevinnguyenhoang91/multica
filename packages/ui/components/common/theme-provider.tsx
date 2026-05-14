import { useCallback, useEffect, useMemo, useState } from "react";
import { ThemeProvider as NextThemesProvider, useTheme as useNextTheme } from "next-themes";
import { TooltipProvider } from "../ui/tooltip";

const ACCENT_STORAGE_PREFIX = "multica:accent:";
const ACCENT_VALUES = [
  "default",
  "blue",
  "purple",
  "pink",
  "red",
  "orange",
  "yellow",
  "green",
  "teal",
] as const;
type ResolvedTheme = "light" | "dark";
export type AccentColor = (typeof ACCENT_VALUES)[number];

function isAccentColor(value: string | null): value is AccentColor {
  return value !== null && (ACCENT_VALUES as readonly string[]).includes(value);
}

function storageKey(theme: ResolvedTheme): string {
  return `${ACCENT_STORAGE_PREFIX}${theme}`;
}

function readStoredAccent(theme: ResolvedTheme): AccentColor {
  const raw = window.localStorage.getItem(storageKey(theme));
  return isAccentColor(raw) ? raw : "default";
}

function writeStoredAccent(theme: ResolvedTheme, accent: AccentColor): void {
  window.localStorage.setItem(storageKey(theme), accent);
}

function applyAccentToRoot(accent: AccentColor): void {
  const root = document.documentElement;
  if (accent === "default") {
    root.style.removeProperty("--accent");
    root.style.removeProperty("--accent-foreground");
    return;
  }
  root.style.setProperty("--accent", `var(--accent-${accent})`);
  root.style.setProperty("--accent-foreground", `var(--accent-${accent}-foreground)`);
}

export function useTheme() {
  const nextTheme = useNextTheme();
  const [accent, setAccentState] = useState<AccentColor>("default");

  const currentResolvedTheme = useMemo<ResolvedTheme | null>(() => {
    if (nextTheme.resolvedTheme === "light" || nextTheme.resolvedTheme === "dark") {
      return nextTheme.resolvedTheme;
    }
    return null;
  }, [nextTheme.resolvedTheme]);

  useEffect(() => {
    if (typeof window === "undefined" || currentResolvedTheme === null) return;
    const currentAccent = readStoredAccent(currentResolvedTheme);
    setAccentState(currentAccent);
    applyAccentToRoot(currentAccent);
  }, [currentResolvedTheme]);

  const setAccent = useCallback(
    (nextAccent: AccentColor) => {
      if (typeof window === "undefined" || currentResolvedTheme === null) return;
      writeStoredAccent(currentResolvedTheme, nextAccent);
      setAccentState(nextAccent);
      applyAccentToRoot(nextAccent);
    },
    [currentResolvedTheme],
  );

  return {
    ...nextTheme,
    accent,
    setAccent,
  };
}

export function ThemeProvider({
  children,
  ...props
}: React.ComponentProps<typeof NextThemesProvider>) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
      {...props}
    >
      <TooltipProvider delay={500}>
        {children}
      </TooltipProvider>
    </NextThemesProvider>
  );
}
