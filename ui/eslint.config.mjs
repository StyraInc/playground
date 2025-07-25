import js from "@eslint/js";
import globals from "globals";

export default [
  js.configs.recommended,
  {
    languageOptions: {
      ecmaVersion: 2018,
      sourceType: 'module',
      globals: {
        ...globals.es2015,
        ...globals.browser,
        Atomics: 'readonly',
        SharedArrayBuffer: 'readonly',
      }
    },
    rules: {
      'indent': [
        'error',
        2,
        {
          'MemberExpression': 'off',
          'SwitchCase': 1
        }
      ],
      'linebreak-style': [
        'error',
        'unix'
      ],
      'quotes': [
        'error',
        'single'
      ],
      'semi': [
        'error',
        'never'
      ]
    }
  }
]
