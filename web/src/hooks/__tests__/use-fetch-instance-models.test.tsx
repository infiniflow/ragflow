import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import React from 'react';

import { useFetchInstanceModels } from '../use-llm-request';

jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: { listInstanceModels: jest.fn() },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('../use-warn-empty-model', () => ({
  useWarnEmptyModel: jest.fn(),
}));

import llmService from '@/services/llm-service';

const mockListInstanceModels = jest.mocked(llmService.listInstanceModels);

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // esbuild-jest cannot transform React when the same import is also used
  // as a type namespace in this repository's test configuration.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const Wrapper = (props: { children: any }) =>
    React.createElement(
      QueryClientProvider,
      { client: queryClient },
      props.children,
    );
  return Wrapper;
}

describe('useFetchInstanceModels', () => {
  it('keeps data undefined until persisted models are loaded', async () => {
    let resolveRequest: (value: unknown) => void = () => undefined;
    mockListInstanceModels.mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve;
      }) as never,
    );

    const { result } = renderHook(
      () => useFetchInstanceModels('Bedrock', 'saved-instance'),
      { wrapper: makeWrapper() },
    );

    expect(result.current.data).toBeUndefined();

    await act(async () => {
      resolveRequest({
        data: {
          data: [{ name: 'model-a', model_type: 'chat', max_tokens: 8192 }],
        },
      });
    });

    await waitFor(() =>
      expect(result.current.data?.map((model) => model.name)).toEqual([
        'model-a',
      ]),
    );
  });
});
