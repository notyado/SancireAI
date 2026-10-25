import js from "@eslint/js";
import tseslint from "typescript-eslint";
export default tseslint.config(
  { ignores: ["dist/**", "node_modules/**"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    rules: {
      "no-unused-vars": "off",
      "no-empty": "off",
      "@typescript-eslint/no-unused-vars": "off",
      "@typescript-eslint/no-explicit-any": "off"
    },
    languageOptions: { globals: { window: "readonly", document: "readonly", localStorage: "readonly", sessionStorage: "readonly", fetch: "readonly", console: "readonly", crypto: "readonly", URL: "readonly", setTimeout: "readonly", clearTimeout: "readonly", requestAnimationFrame: "readonly", cancelAnimationFrame: "readonly" } }
  },
);
