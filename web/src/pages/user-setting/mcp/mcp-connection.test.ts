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

it.each(['authorization_token', 'Authorization_Token'])(
  'removes the Python-imported %s credential when the token is changed or cleared',
  (header) => {
    const saved = {
      headers: { [header]: 'old-secret', 'User-Agent': 'ragflow' },
      variables: { authorization_token: 'old-secret' },
    };
    expect(connectionFields(TokenMask, saved)).toEqual(saved);
    expect(connectionFields('', saved)).toEqual({
      headers: { 'User-Agent': 'ragflow' },
      variables: { authorization_token: '' },
    });
    expect(connectionFields('new-secret', saved)).toEqual({
      headers: {
        'User-Agent': 'ragflow',
        Authorization: 'Bearer ${authorization_token}',
      },
      variables: { authorization_token: 'new-secret' },
    });
  },
);

it.each(['${auth_header}', '$auth_header', '${prefix}ization', '${go header}'])(
  'removes authentication resolved from %s while preserving unrelated templates',
  (header) => {
    const saved = {
      headers: {
        [header]: 'Bearer old-secret',
        '${agent_header}': '${agent}',
        $$literal: 'keep',
      },
      variables: {
        authorization_token: 'old-secret',
        auth_header: 'Authorization',
        prefix: 'Author',
        'go header': 'Authorization',
        agent_header: 'User-Agent',
        agent: 'ragflow',
      },
    };
    expect(connectionFields(TokenMask, saved)).toEqual(saved);
    expect(connectionFields('', saved).headers).toEqual({
      '${agent_header}': '${agent}',
      $$literal: 'keep',
    });
    expect(connectionFields('new-secret', saved).headers).toEqual({
      '${agent_header}': '${agent}',
      $$literal: 'keep',
      Authorization: 'Bearer ${authorization_token}',
    });
  },
);

it('classifies token-dependent names before and after replacement without recursive expansion', () => {
  const saved = {
    headers: {
      '${authorization_token}': 'old-secret',
      '$${auth}': 'keep',
      '${alias}': 'keep',
    },
    variables: {
      authorization_token: 'X-Old',
      auth: 'Authorization',
      alias: '${auth}',
    },
  };
  expect(connectionFields('Authorization', saved).headers).toEqual({
    '$${auth}': 'keep',
    '${alias}': 'keep',
    Authorization: 'Bearer ${authorization_token}',
  });
});
