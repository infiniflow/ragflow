// Editing an imported server must not replace its authentication scheme or
// custom headers unless the user actually changes the token field.
export const TokenMask = '********';

type Connection = {
  headers?: Record<string, string>;
  variables?: Record<string, unknown>;
};

// Resolve arbitrary ${name} and bare $name placeholders against the variables
// to classify the header without rewriting unrelated templates.
function isAuthorizationHeader(
  key: string,
  variables: Record<string, unknown>,
) {
  const lookup = (match: string, name: string) =>
    Object.prototype.hasOwnProperty.call(variables, name)
      ? String(variables[name])
      : match;
  // Braced forms first, then bare $name (require a word-leading name so
  // "$$literal" is not treated as a placeholder).
  const resolved = key
    .replace(/\$\{([^}]*)\}/g, lookup)
    .replace(/\$([A-Za-z][\w ]*)/g, lookup);
  return [key, resolved].some((name) =>
    ['authorization', 'authorization_token'].includes(name.toLowerCase()),
  );
}

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
      if (
        isAuthorizationHeader(key, current.variables ?? {}) ||
        isAuthorizationHeader(key, {
          ...current.variables,
          authorization_token: token,
        })
      ) {
        delete headers[key];
      }
    }
    if (token) headers.Authorization = 'Bearer ${authorization_token}';
  }
  return {
    variables: { ...current.variables, authorization_token: token },
    headers,
  };
}
