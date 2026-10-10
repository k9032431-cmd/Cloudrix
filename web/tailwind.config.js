// Colours come from the slate design tokens in src/slate.css, so every
// utility follows light/dark mode and the theme preset. color-mix keeps
// opacity modifiers (bg-card/80) working with oklch tokens.
const tok = (name) => `color-mix(in oklab, var(--${name}) calc(<alpha-value> * 100%), transparent)`

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Geist Variable"', 'Geist', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['"Geist Mono Variable"', '"Geist Mono"', 'ui-monospace', 'monospace'],
      },
      colors: {
        background: tok('background'),
        foreground: tok('foreground'),
        card: { DEFAULT: tok('card'), foreground: tok('card-foreground') },
        popover: { DEFAULT: tok('popover'), foreground: tok('popover-foreground') },
        primary: { DEFAULT: tok('primary'), foreground: tok('primary-foreground') },
        secondary: { DEFAULT: tok('secondary'), foreground: tok('secondary-foreground') },
        muted: { DEFAULT: tok('muted'), foreground: tok('muted-foreground') },
        faint: tok('faint-foreground'),
        accent: { DEFAULT: tok('accent'), foreground: tok('accent-foreground') },
        border: { DEFAULT: tok('border'), strong: tok('border-strong') },
        input: tok('input'),
        ring: tok('ring'),
        sidebar: { DEFAULT: tok('sidebar'), accent: tok('sidebar-accent'), border: tok('sidebar-border') },
        chart: { 1: tok('chart-1'), 2: tok('chart-2'), 3: tok('chart-3'), 4: tok('chart-4'), 5: tok('chart-5') },
        success: tok('success'),
        warning: tok('warning'),
        destructive: tok('destructive'),
        violet: tok('violet'),
      },
      borderRadius: { panel: '0.875rem', card: '0.625rem' },
      boxShadow: { xs: 'var(--shadow-xs)', sm: 'var(--shadow-sm)', md: 'var(--shadow-md)', lg: 'var(--shadow-lg)' },
      transitionTimingFunction: { out: 'cubic-bezier(0.23, 1, 0.32, 1)' },
    },
  },
  plugins: [],
}
