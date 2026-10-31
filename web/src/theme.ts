// Palettes are derived from the MACE logo: signal red #A20000, ink, paper.
// data-palette selects the palette; data-theme keeps the light/dark scheme
// that older styles key on.

export type Palette = "signal" | "ink" | "ember" | "carbon";
export type ThemePref = Palette | "auto";

export const palettes: {
  id: Palette;
  name: string;
  scheme: "light" | "dark";
  note: string;
}[] = [
  { id: "signal", name: "Signal", scheme: "light", note: "Paper, ink, and a red signal." },
  { id: "ink", name: "Ink", scheme: "dark", note: "Dark and quiet for long sessions." },
  { id: "ember", name: "Ember", scheme: "light", note: "Red forward, warm paper." },
  { id: "carbon", name: "Carbon", scheme: "dark", note: "Maximum contrast." },
];

const read = (key: string) => {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
};
const write = (key: string, value: string) => {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Private windows can refuse storage; the theme still applies for this visit. */
  }
};

export function themePref(): ThemePref {
  const saved = read("mace-palette");
  if (saved && (saved === "auto" || palettes.some((p) => p.id === saved)))
    return saved as ThemePref;
  // Earlier versions stored a light/dark choice.
  const legacy = read("mace-theme");
  if (legacy === "dark") return "ink";
  if (legacy === "light") return "signal";
  return "auto";
}

const systemDark = () =>
  typeof matchMedia === "function" && matchMedia("(prefers-color-scheme: dark)").matches;

export function resolve(pref: ThemePref): Palette {
  return pref === "auto" ? (systemDark() ? "ink" : "signal") : pref;
}

export function applyTheme(pref: ThemePref) {
  const palette = resolve(pref);
  const scheme = palettes.find((p) => p.id === palette)!.scheme;
  const root = document.documentElement;
  root.dataset.palette = palette;
  root.dataset.theme = scheme;
  root.style.colorScheme = scheme;
  write("mace-palette", pref);
  const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
  if (meta) meta.content = getComputedStyle(root).getPropertyValue("--chrome").trim() || "#0e0b0b";
  return palette;
}

export function wallpaperPref() {
  return read("mace-wallpaper") !== "plain";
}
export function setWallpaper(on: boolean) {
  write("mace-wallpaper", on ? "pattern" : "plain");
  document.documentElement.dataset.wallpaper = on ? "pattern" : "plain";
}
