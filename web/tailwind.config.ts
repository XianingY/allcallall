import type { Config } from "tailwindcss";
import forms from "@tailwindcss/forms";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#17211c",
        muted: "#5d6a63",
        line: "#d9e4de",
        canvas: "#f3f7f4",
        panel: "#ffffff",
        brand: "#12372a",
        accent: "#e9a83a",
        danger: "#b42318"
      },
      boxShadow: { panel: "0 1px 2px rgba(16,24,40,.06)" },
    },
  },
  plugins: [forms],
} satisfies Config;
