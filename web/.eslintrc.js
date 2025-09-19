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
