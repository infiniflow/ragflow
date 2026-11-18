const NativeDate = globalThis.Date;
const ClientOffsetMilliseconds = 6 * 60 * 60 * 1000;

class ClientTimezoneDate extends NativeDate {
  private inClientZone() {
    return new NativeDate(this.getTime() + ClientOffsetMilliseconds);
  }

  getTimezoneOffset() {
    return -360;
  }

  getFullYear() {
    return this.inClientZone().getUTCFullYear();
  }

  getMonth() {
    return this.inClientZone().getUTCMonth();
  }

  getDate() {
    return this.inClientZone().getUTCDate();
  }

  getDay() {
    return this.inClientZone().getUTCDay();
  }

  getHours() {
    return this.inClientZone().getUTCHours();
  }

  getMinutes() {
    return this.inClientZone().getUTCMinutes();
  }

  getSeconds() {
    return this.inClientZone().getUTCSeconds();
  }

  getMilliseconds() {
    return this.inClientZone().getUTCMilliseconds();
  }
}

globalThis.Date = ClientTimezoneDate as unknown as DateConstructor;

const {
  formatDate,
  formatPureDate,
  formatSecondsToHumanReadable,
  formatStandardDate,
  formatTime,
} = jest.requireActual('../date') as typeof import('../date');

afterAll(() => {
  globalThis.Date = NativeDate;
});

describe('date display formatting', () => {
  const timestamp = Date.UTC(2026, 10, 13, 7, 25, 7);

  test.each([
    ['an offset-bearing string', '2026-11-13T15:25:07+08:00'],
    ['a UTC string', '2026-11-13T07:25:07Z'],
    ['an epoch-millisecond timestamp', timestamp],
  ])('formats %s in the client timezone', (_, value) => {
    expect(formatDate(value)).toBe('2026-11-13 13:25:07');
  });

  test('uses the client date when timezone conversion crosses midnight', () => {
    expect(formatDate('2026-11-13T23:30:00-05:00')).toBe('2026-11-14 10:30:00');
  });

  test('formats date-only and time-only displays with their own precision', () => {
    expect(formatPureDate(timestamp)).toBe('2026-11-13');
    expect(formatStandardDate(timestamp)).toBe('2026-11-13');
    expect(formatTime(timestamp)).toBe('13:25:07');
  });
});

describe('formatSecondsToHumanReadable', () => {
  test('formats sub-minute durations in seconds', () => {
    expect(formatSecondsToHumanReadable(0)).toBe('0s');
    expect(formatSecondsToHumanReadable(30)).toBe('30s');
  });

  test('strips trailing zeros from fractional seconds', () => {
    expect(formatSecondsToHumanReadable(1.5)).toBe('1.5s');
  });

  test('does not emit a trailing space for whole minutes or hours', () => {
    expect(formatSecondsToHumanReadable(60)).toBe('1m');
    expect(formatSecondsToHumanReadable(3600)).toBe('1h');
    expect(formatSecondsToHumanReadable(7200)).toBe('2h');
  });

  test('does not emit a trailing space when seconds are zero', () => {
    expect(formatSecondsToHumanReadable(3660)).toBe('1h 1m');
    expect(formatSecondsToHumanReadable(5400)).toBe('1h 30m');
  });

  test('joins non-zero hour/minute/second parts with a single space', () => {
    expect(formatSecondsToHumanReadable(90)).toBe('1m 30s');
    expect(formatSecondsToHumanReadable(125)).toBe('2m 5s');
    expect(formatSecondsToHumanReadable(3661)).toBe('1h 1m 1s');
  });

  test('returns "0s" for negative or non-numeric input', () => {
    expect(formatSecondsToHumanReadable(-5)).toBe('0s');
    expect(formatSecondsToHumanReadable(NaN)).toBe('0s');
  });
});
