const chuhoman = require('@chuhoman/eslint-config').default;

module.exports = chuhoman(
  {},
  {
    rules: {
      'no-console': 'off',
      'prefer-regex-literals': 'off',
      '@typescript-eslint/consistent-type-assertions': 'off',
      'no-undef': 'off',
      'ts/type-annotation-spacing': ['error', {}],
    },
  },
);
