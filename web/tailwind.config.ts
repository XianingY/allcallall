import type { Config } from "tailwindcss";
import forms from "@tailwindcss/forms";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#17202a",
        muted: "#667085",
        line: "#dfe3e8",
        canvas: "#f7f8fa",
        panel: "#ffffff",
        brand: "#146c5a",
        accent: "#d97706",
        danger: "#b42318"
      },
      boxShadow: { panel: "0 1px 2px rgba(16,24,40,.06)" },
    },
  },
  plugins: [forms],
} satisfies Config;
