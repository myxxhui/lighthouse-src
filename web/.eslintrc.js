module.exports = {
  extends: ['react-app', 'react-app/jest'],
  rules: {
    'no-unused-vars': 'error',
    'no-undef': 'error',
    'react-hooks/rules-of-hooks': 'error',
    'react-hooks/exhaustive-deps': 'warn',
    '@typescript-eslint/no-unused-vars': 'error',
    'import/no-unresolved': 'off',
    'import/no-extraneous-dependencies': 'off',
  },
};