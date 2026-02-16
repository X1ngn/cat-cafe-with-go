/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: '#ccd9e5',
        'cat-orange': '#ff9966',
        'cat-beige': '#d9bf99',
        'user-green': '#d9f2e5',
        'bg-cream': '#faf5f0',
        'status-idle': '#ccf2cc',
        'status-busy': '#ffd9cc',
      },
    },
  },
  plugins: [],
}
