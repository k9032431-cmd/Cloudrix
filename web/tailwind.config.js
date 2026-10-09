/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: { sans: ['Inter', 'system-ui', 'sans-serif'] },
      colors: {
        brand: {
          50: '#eef6ff', 100: '#d9eaff', 200: '#bcdaff', 300: '#8ec3ff', 400: '#59a2ff',
          500: '#337ffc', 600: '#1d60f1', 700: '#154ade', 800: '#183db4', 900: '#1a378d', 950: '#152356',
        },
      },
    },
  },
  plugins: [],
}
