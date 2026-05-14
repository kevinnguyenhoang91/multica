"use client";

import { toast } from "sonner";
import {
  useTheme,
  type AccentColor,
} from "@multica/ui/components/common/theme-provider";
import { cn } from "@multica/ui/lib/utils";
import {
  DEFAULT_LOCALE,
  SUPPORTED_LOCALES,
  type SupportedLocale,
} from "@multica/core/i18n";
import { useLocaleAdapter } from "@multica/core/i18n/react";
import { useAuthStore } from "@multica/core/auth";
import { api } from "@multica/core/api";
import { useT } from "../../i18n";

const LIGHT_COLORS = {
  titleBar: "#e8e8e8",
  content: "#ffffff",
  sidebar: "#f4f4f5",
  bar: "#e4e4e7",
  barMuted: "#d4d4d8",
};

const DARK_COLORS = {
  titleBar: "#333338",
  content: "#27272a",
  sidebar: "#1e1e21",
  bar: "#3f3f46",
  barMuted: "#52525b",
};

const ACCENT_OPTIONS: {
  value: AccentColor;
  labelKey:
    | "default"
    | "blue"
    | "purple"
    | "pink"
    | "red"
    | "orange"
    | "yellow"
    | "green"
    | "teal";
  swatchClass: string;
}[] = [
  {
    value: "default",
    labelKey: "default",
    swatchClass: "bg-muted border-border",
  },
  { value: "blue", labelKey: "blue", swatchClass: "bg-accent-blue/50" },
  { value: "purple", labelKey: "purple", swatchClass: "bg-accent-purple/50" },
  { value: "pink", labelKey: "pink", swatchClass: "bg-accent-pink/50" },
  { value: "red", labelKey: "red", swatchClass: "bg-accent-red/50" },
  { value: "orange", labelKey: "orange", swatchClass: "bg-accent-orange/50" },
  { value: "yellow", labelKey: "yellow", swatchClass: "bg-accent-yellow/50" },
  { value: "green", labelKey: "green", swatchClass: "bg-accent-green/50" },
  { value: "teal", labelKey: "teal", swatchClass: "bg-accent-teal/50" },
];

function WindowMockup({
  variant,
  className,
}: {
  variant: "light" | "dark";
  className?: string;
}) {
  const colors = variant === "light" ? LIGHT_COLORS : DARK_COLORS;

  return (
    <div className={cn("flex h-full w-full flex-col", className)}>
      {/* Title bar */}
      <div
        className="flex items-center gap-[3px] px-2 py-1.5"
        style={{ backgroundColor: colors.titleBar }}
      >
        <span className="size-[6px] rounded-full bg-[#ff5f57]" />
        <span className="size-[6px] rounded-full bg-[#febc2e]" />
        <span className="size-[6px] rounded-full bg-[#28c840]" />
      </div>
      {/* Content area */}
      <div
        className="flex flex-1"
        style={{ backgroundColor: colors.content }}
      >
        {/* Sidebar */}
        <div
          className="w-[30%] space-y-1 p-2"
          style={{ backgroundColor: colors.sidebar }}
        >
          <div
            className="h-1 w-3/4 rounded-full"
            style={{ backgroundColor: colors.bar }}
          />
          <div
            className="h-1 w-1/2 rounded-full"
            style={{ backgroundColor: colors.bar }}
          />
        </div>
        {/* Main */}
        <div className="flex-1 space-y-1.5 p-2">
          <div
            className="h-1.5 w-4/5 rounded-full"
            style={{ backgroundColor: colors.bar }}
          />
          <div
            className="h-1 w-full rounded-full"
            style={{ backgroundColor: colors.barMuted }}
          />
          <div
            className="h-1 w-3/5 rounded-full"
            style={{ backgroundColor: colors.barMuted }}
          />
        </div>
      </div>
    </div>
  );
}

export function PreferencesTab() {
  const { theme, setTheme, resolvedTheme, accent, setAccent } = useTheme();
  const { t, i18n } = useT("settings");
  const localeAdapter = useLocaleAdapter();
  const user = useAuthStore((s) => s.user);

  // i18next.language can be a region-tagged BCP-47 string (e.g. "en-US",
  // "zh-Hans-CN") returned by intl-localematcher. Normalize to a supported
  // locale before comparing — otherwise the radio shows neither option active.
  const currentLocale: SupportedLocale = SUPPORTED_LOCALES.includes(
    i18n.language as SupportedLocale,
  )
    ? (i18n.language as SupportedLocale)
    : DEFAULT_LOCALE;

  const themeOptions = [
    { value: "light" as const, label: t(($) => $.preferences.theme.light) },
    { value: "dark" as const, label: t(($) => $.preferences.theme.dark) },
    { value: "system" as const, label: t(($) => $.preferences.theme.system) },
  ];

  const languageOptions: { value: SupportedLocale; label: string }[] = [
    { value: "en", label: t(($) => $.preferences.language.english) },
    { value: "zh-Hans", label: t(($) => $.preferences.language.chinese) },
  ];
  const activeResolvedTheme = resolvedTheme === "dark" ? "dark" : "light";
  const activeResolvedThemeLabel = t(
    ($) => $.preferences.theme[activeResolvedTheme],
  );

  // Persist locally → sync to user.language → reload. Reload (vs in-place
  // changeLanguage) avoids hydration mismatch and is the i18next-recommended
  // pattern for App Router.
  //
  // If the cross-device sync (PATCH /api/me) fails, the local cookie is
  // already written so the new locale will take effect after reload — but
  // the user's other devices won't see the change. Surface that explicitly
  // via a toast and delay the reload long enough for the toast to be read,
  // otherwise the failure would be invisible.
  const handleLanguageChange = async (next: SupportedLocale) => {
    if (next === currentLocale) return;
    localeAdapter.persist(next);

    let syncFailed = false;
    if (user) {
      try {
        await api.updateMe({ language: next });
      } catch {
        syncFailed = true;
      }
    }

    if (syncFailed) {
      toast.warning(t(($) => $.preferences.language.sync_failed));
      // Give the toast 2.5s of visible time before navigating away.
      setTimeout(() => window.location.reload(), 2500);
      return;
    }
    window.location.reload();
  };

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <h2 className="text-sm font-semibold">
          {t(($) => $.preferences.theme.title)}
        </h2>
        <div className="flex gap-6" role="radiogroup">
          {themeOptions.map((opt) => {
            const active = theme === opt.value;
            return (
              <button
                key={opt.value}
                role="radio"
                aria-checked={active}
                onClick={() => setTheme(opt.value)}
                className="group flex flex-col items-center gap-2"
              >
                <div
                  className={cn(
                    "aspect-[4/3] w-36 overflow-hidden rounded-lg ring-1 transition-all",
                    active
                      ? "ring-2 ring-brand"
                      : "ring-border hover:ring-2 hover:ring-border"
                  )}
                >
                  {opt.value === "system" ? (
                    <div className="relative h-full w-full">
                      <WindowMockup
                        variant="light"
                        className="absolute inset-0"
                      />
                      <WindowMockup
                        variant="dark"
                        className="absolute inset-0 [clip-path:inset(0_0_0_50%)]"
                      />
                    </div>
                  ) : (
                    <WindowMockup variant={opt.value} />
                  )}
                </div>
                <span
                  className={cn(
                    "text-sm transition-colors",
                    active
                      ? "font-medium text-foreground"
                      : "text-muted-foreground"
                  )}
                >
                  {opt.label}
                </span>
              </button>
            );
          })}
        </div>
      </section>

      <section className="space-y-4">
        <h2 className="text-sm font-semibold">
          {t(($) => $.preferences.language.title)}
        </h2>
        <div className="flex gap-3" role="radiogroup">
          {languageOptions.map((opt) => {
            const active = currentLocale === opt.value;
            return (
              <button
                key={opt.value}
                role="radio"
                aria-checked={active}
                onClick={() => handleLanguageChange(opt.value)}
                className={cn(
                  "rounded-md border px-4 py-2 text-sm transition-colors",
                  active
                    ? "border-brand bg-brand/10 font-medium text-foreground"
                    : "border-border text-muted-foreground hover:border-foreground/30"
                )}
              >
                {opt.label}
              </button>
            );
          })}
        </div>
      </section>

      <section className="space-y-4">
        <div className="space-y-1">
          <h2 className="text-sm font-semibold">
            {t(($) => $.preferences.theme.accent_title)}
          </h2>
          <p className="text-xs text-muted-foreground">
            {t(($) => $.preferences.theme.accent_current_theme, {
              theme: activeResolvedThemeLabel,
            })}
          </p>
        </div>
        <div className="flex flex-wrap gap-3" role="radiogroup">
          {ACCENT_OPTIONS.map((opt) => {
            const active = accent === opt.value;
            return (
              <button
                key={opt.value}
                role="radio"
                aria-checked={active}
                onClick={() => setAccent(opt.value)}
                className={cn(
                  "flex items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors",
                  opt.value === "default"
                    ? "border-border text-foreground hover:border-foreground/30"
                    : active
                    ? "border-brand bg-brand/10 font-medium text-foreground"
                    : "border-border text-muted-foreground hover:border-foreground/30",
                )}
              >
                <span
                  aria-hidden
                  className={cn(
                    "size-3 rounded-full border border-black/10",
                    opt.swatchClass,
                  )}
                />
                {t(($) => $.preferences.theme.accent_options[opt.labelKey])}
              </button>
            );
          })}
        </div>
      </section>
    </div>
  );
}
