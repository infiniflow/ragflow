// Editing an imported server must not replace its authentication scheme or
// custom headers unless the user actually changes the token field.
export const TokenMask = '********';

type Connection = {
  headers?: Record<string, string>;
  variables?: Record<string, unknown>;
};

export function connectionFields(
  value: string | undefined,
  current: Connection,
) {
  const storedToken = current.variables?.authorization_token;
  const token = value === TokenMask ? storedToken : value;
  const unchanged =
    value === TokenMask || (token || '') === (storedToken || '');
  const headers = { ...current.headers };
  if (!unchanged) {
    for (const key of Object.keys(headers)) {
      if (key.toLowerCase() === 'authorization') delete headers[key];
    }
    if (token) headers.Authorization = 'Bearer ${authorization_token}';
  }
  return {
    variables: { ...current.variables, authorization_token: token },
    headers,
  };
}
