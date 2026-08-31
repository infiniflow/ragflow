import { history } from '../simple-history-util';

describe('simple-history-util history.location', () => {
  afterEach(() => {
    window.history.replaceState(null, '', '/');
  });

  it('exposes the current path segments from window.location', () => {
    window.history.replaceState(null, '', '/foo?bar=1#baz');

    expect(history.location.pathname).toBe('/foo');
    expect(history.location.search).toBe('?bar=1');
    expect(history.location.hash).toBe('#baz');
  });

  it('reflects the real browser navigation state after replace()', () => {
    const state = { token: 'abc' };
    history.replace('/session', state);

    expect(window.history.state).toEqual(state);
    // Regression: the getter previously read the GlobalHistory instance's
    // own (never-assigned) `state` field instead of window.history.state,
    // so it always returned undefined.
    expect(history.location.state).toEqual(state);
  });

  it('reflects the real browser navigation state after push()', () => {
    const state = { from: '/prev' };
    history.push('/next', state);

    expect(history.location.state).toEqual(state);
    expect(history.location.state).toEqual(window.history.state);
  });
});
