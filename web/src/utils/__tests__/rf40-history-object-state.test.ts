import { history } from '../simple-history-util';
describe('object destination state', () => {
  afterEach(() => window.history.replaceState(null, '', '/'));
  test.each(['push', 'replace'] as const)(
    '%s preserves destination state',
    (method) => {
      history[method]({
        pathname: '/session',
        search: '?view=chat',
        hash: '#answer',
        state: { from: 'object' },
      });
      expect(history.location).toEqual({
        pathname: '/session',
        search: '?view=chat',
        hash: '#answer',
        state: { from: 'object' },
      });
    },
  );
  test.each(['push', 'replace'] as const)(
    '%s preserves explicit state precedence',
    (method) => {
      history[method](
        { pathname: '/session', state: { from: 'object' } },
        { from: 'argument' },
      );
      expect(history.location.state).toEqual({ from: 'argument' });
    },
  );
  test.each(['push', 'replace'] as const)(
    '%s honors explicit null state',
    (method) => {
      history[method](
        { pathname: '/session', state: { from: 'object' } },
        null,
      );
      expect(history.location.state).toBeNull();
    },
  );
  test('preserves string destination state', () => {
    history.push('/next', { from: 'string' });
    expect(history.location.state).toEqual({ from: 'string' });
  });
});
