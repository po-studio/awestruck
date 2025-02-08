/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'neon-pink': '#FF0080',
        'gray': {
          800: '#1a1a1a',
          900: '#111111',
        }
      },
      fontFamily: {
        'bungee': ['Bungee Shade', 'cursive'],
        'space': ['Space Grotesk', 'sans-serif'],
        'mono': ['IBM Plex Mono', 'monospace']
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out forwards',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        }
      }
    },
  },
  plugins: [],
} 