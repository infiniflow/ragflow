// .eslintrc.js
module.exports = {
  extends: [require.resolve('umi/eslint'), 'plugin:react-hooks/recommended'],
  plugins: ['check-file'],
  rules: {
    '@typescript-eslint/no-use-before-define': [
      'warn',
      {
        functions: false,
        variables: true,
      },
    ],
    'check-file/filename-naming-convention': [
      'error',
      {
        '**/*.{jsx,tsx}': 'KEBAB_CASE',
        '**/*.{js,ts}': 'KEBAB_CASE',
      },
    ],
    'check-file/folder-naming-convention': [
      'error',
      {
        'src/**/': 'KEBAB_CASE',
        'mocks/*/': 'KEBAB_CASE',
      },
    ],
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
  },
};
