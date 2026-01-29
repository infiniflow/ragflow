module.exports = {
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react/recommended',
    'plugin:react-hooks/recommended',
  ],
  plugins: ['@typescript-eslint', 'react', 'react-refresh', 'check-file'],
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
    ecmaFeatures: {
      jsx: true,
    },
  },
  settings: {
    react: {
      version: 'detect',
    },
  },
  env: {
    browser: true,
    es2021: true,
    node: true,
  },
  rules: {
    '@typescript-eslint/no-use-before-define': [
      'warn',
      {
        functions: false,
        variables: true,
      },
    ],
    '@typescript-eslint/no-explicit-any': 'off',
    '@typescript-eslint/ban-ts-comment': 'off',
    '@typescript-eslint/no-empty-function': 'off',
    '@typescript-eslint/no-non-null-assertion': 'off',
    'react/prop-types': 'off',
    'react/react-in-jsx-scope': 'off',
    'react/no-unescaped-entities': [
      'warn',
      {
        forbid: [
          {
            char: "'",
            alternatives: ['&apos;', '&#39;'],
          },
          {
            char: '"',
            alternatives: ['&quot;', '&#34;'],
          },
        ],
      },
    ],
    'react-refresh/only-export-components': 'off',
    'no-console': ['warn', { allow: ['warn', 'error'] }],
    'check-file/filename-naming-convention': [
      'error',
      {
        '**/*.{jsx,tsx}': '[a-z0-9.-]*',
        '**/*.{js,ts}': '[a-z0-9.-]*',
      },
    ],
    'check-file/folder-naming-convention': [
      'error',
      {
        'src/**/': 'KEBAB_CASE',
        'mocks/*/': 'KEBAB_CASE',
      },
    ],
  },
};
