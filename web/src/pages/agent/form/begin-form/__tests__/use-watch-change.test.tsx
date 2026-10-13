import { renderHook } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { useWatchFormChange } from '../use-watch-change';

const mockUpdateNodeForm = jest.fn();

jest.mock('../../../store', () => ({
  __esModule: true,
  default: (selector: any) => selector({ updateNodeForm: mockUpdateNodeForm }),
}));

test('exposes webhook section fields as Begin output references', () => {
  renderHook(() => {
    const form = useForm<Record<string, any>>({
      defaultValues: {
        mode: 'Webhook',
        schema: {
          headers: [{ key: 'x-request-id', type: 'string', required: true }],
          query: [{ key: 'q', type: 'string', required: false }],
          body: [{ key: 'value', type: 'string', required: false }],
        },
      },
    });
    useWatchFormChange('Begin', form);
  });
  expect(mockUpdateNodeForm).toHaveBeenCalledWith(
    'Begin',
    expect.objectContaining({
      outputs: {
        'headers.x-request-id': { type: 'string' },
        'query.q': { type: 'string' },
        'body.value': { type: 'string' },
      },
    }),
  );
});
