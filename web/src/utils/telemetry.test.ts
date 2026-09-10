import { Authorization } from '@/constants/authorization';
import { installClientTelemetry } from './telemetry';

describe('client telemetry', () => {
  afterEach(() => {
    localStorage.clear();
    jest.restoreAllMocks();
  });

  it('ignores ResizeObserver delivery warnings but reports real errors', () => {
    localStorage.setItem(Authorization, 'Bearer telemetry-test');
    const fetchMock = jest
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 204 }));
    installClientTelemetry();

    window.dispatchEvent(
      new ErrorEvent('error', {
        message:
          'ResizeObserver loop completed with undelivered notifications.',
      }),
    );
    expect(fetchMock).not.toHaveBeenCalled();

    window.dispatchEvent(
      new ErrorEvent('error', {
        error: new Error('real failure'),
        message: 'real failure',
      }),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/system/client-errors',
      expect.objectContaining({ method: 'POST' }),
    );
  });
});
