import { connectionFields, TokenMask } from './mcp-connection';

it('preserves imported headers and template variables without mutating the saved connection', () => {
  const saved = {
    headers: { 'User-Agent': 'ragflow', 'X-Token': '${custom}' },
    variables: { custom: 'secret', name: 'header-identity' },
  };
  const before = JSON.stringify(saved);
  expect(connectionFields('', saved)).toEqual({
    headers: saved.headers,
    variables: { ...saved.variables, authorization_token: '' },
  });
  expect(JSON.stringify(saved)).toBe(before);
});

it('keeps a masked token and the imported authentication scheme', () => {
  const saved = {
    headers: {
      authorization: 'Token ${authorization_token}',
      'User-Agent': 'ragflow',
    },
    variables: { authorization_token: 'secret' },
  };
  expect(connectionFields(TokenMask, saved)).toEqual(saved);
});

it('changes only authorization when the user explicitly changes or clears the token', () => {
  const saved = {
    headers: {
      authorization: 'Bearer ${authorization_token}',
      'User-Agent': 'ragflow',
    },
    variables: { authorization_token: 'old', other: 'keep' },
  };
  expect(connectionFields('new', saved)).toEqual({
    headers: {
      Authorization: 'Bearer ${authorization_token}',
      'User-Agent': 'ragflow',
    },
    variables: { authorization_token: 'new', other: 'keep' },
  });
  expect(connectionFields('', saved)).toEqual({
    headers: { 'User-Agent': 'ragflow' },
    variables: { authorization_token: '', other: 'keep' },
  });
});

it('does not add an authentication placeholder to a new no-key connection', () => {
  expect(connectionFields('', {})).toEqual({
    headers: {},
    variables: { authorization_token: '' },
  });
});
