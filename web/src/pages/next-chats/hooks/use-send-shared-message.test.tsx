/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { act, renderHook, waitFor } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router';
import { useGetSharedChatSearchParams } from './use-send-shared-message';

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <MemoryRouter
    initialEntries={[
      '/?from=chat&shared_id=conversation-1&data_region=us&data_tier=free',
    ]}
  >
    {children}
  </MemoryRouter>
);

describe('useGetSharedChatSearchParams', () => {
  it('keeps parsed data stable until the search parameters change', async () => {
    const { result, rerender } = renderHook(
      () => ({
        navigate: useNavigate(),
        params: useGetSharedChatSearchParams(),
      }),
      { wrapper },
    );

    const initialData = result.current.params.data;
    expect(initialData).toEqual({ region: 'us', tier: 'free' });

    rerender();
    expect(result.current.params.data).toBe(initialData);

    act(() => {
      result.current.navigate(
        '/?from=chat&shared_id=conversation-1&data_region=eu&data_tier=pro',
      );
    });

    await waitFor(() =>
      expect(result.current.params.data).toEqual({ region: 'eu', tier: 'pro' }),
    );
    expect(result.current.params.data).not.toBe(initialData);
  });
});
