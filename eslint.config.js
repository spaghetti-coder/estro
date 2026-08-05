import globals from "globals";

export default [
  {
    files: ["**/*.js"],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      globals: {
        ...globals.browser,
      },
    },
    rules: {
      "no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", caughtErrorsIgnorePattern: "^_" },
      ],
      "no-console": ["warn", { allow: ["warn", "error"] }],
      "prefer-const": "error",
      "no-var": "error",
      "no-multi-assign": "error",
      "no-else-return": "error",
      "no-nested-ternary": "error",
      "no-sequences": "error",
      "eqeqeq": ["error", "smart"],
      "complexity": ["warn", { max: 15 }],
      "max-depth": ["warn", 4],
      "max-lines-per-function": ["warn", { max: 150, skipBlankLines: true, skipComments: true }],
      "guard-for-in": "error",
      "no-implicit-coercion": "error",
      "no-param-reassign": ["error", { props: false }],
      "no-return-assign": "error",
      "no-throw-literal": "error",
      "prefer-template": "error",
      "no-eval": "error",
      "no-implied-eval": "error",
      "no-new-func": "error",
      "no-new-wrappers": "error",
      "no-with": "error",
      "indent": ["error", 2],
    },
  },
];
