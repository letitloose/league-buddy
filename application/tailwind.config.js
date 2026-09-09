/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./ui/html/**/*.html", "./ui/html/*.html"],
  theme: {
    extend: {
      colors: {
        brand: {
          paper: '#faf9f5',
          surface: '#ffffff',
          ink: '#1c1c1a',
          muted: '#706c60',
          line: '#e3e0d6',
        },
        // The site's one accent color, as a full Tailwind-style ramp so it
        // can be swapped in everywhere the old orange-* shades were used
        // (buttons, links, focus rings, hover borders, badges) — not just
        // one hex value. 600 is the base "Blame the Ball" accent.
        pitch: {
          50: '#f2f7f4',
          100: '#e1ebe4',
          200: '#c3d8cb',
          300: '#9cbfa9',
          400: '#6f9d81',
          500: '#4f8264',
          600: '#3b6b4f',
          700: '#2f5540',
          800: '#274536',
          900: '#21392d',
        },
      },
    },
  },
  plugins: [],
}
