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

import { act, renderHook } from '@testing-library/react';
import { AgentCategory } from '@/constants/agent';
import { useHandleImportJsonFile } from './use-import-json';

jest.mock('@/hooks/common-hooks', () => ({
  useSetModalState: jest.fn(),
}));

jest.mock('@/hooks/use-agent-request', () => ({
  useCreateAgent: jest.fn(),
}));

const mockNavigate = jest.fn();
const mockNavigateToAgent = jest.fn(() => mockNavigate);
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({ navigateToAgent: mockNavigateToAgent }),
}));

jest.mock('@/components/hooks/use-toast', () => ({
  useToast: () => ({ toast: jest.fn() }),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock('@/pages/agent/utils/dsl-bridge', () => ({
  importDsl: jest.fn(() => ({ graph: {} })),
  inferIsAgentFromImport: jest.fn(),
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockCommonHooks = require('@/hooks/common-hooks');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockAgentRequest = require('@/hooks/use-agent-request');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockDslBridge = require('@/pages/agent/utils/dsl-bridge');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockMessage = require('@/components/ui/message').default;

const createAgent = jest.fn();
const hideFileUploadModal = jest.fn();

function jsonFile(content: unknown): File {
  return {
    type: 'application/json',
    text: async () => JSON.stringify(content),
  } as unknown as File;
}

function uploadPayload(content: unknown) {
  return { fileList: [jsonFile(content)], name: 'Imported' };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockCommonHooks.useSetModalState.mockReturnValue({
    visible: true,
    hideModal: hideFileUploadModal,
    showModal: jest.fn(),
  });
  mockAgentRequest.useCreateAgent.mockReturnValue({
    loading: false,
    createAgent,
  });
});

describe('useHandleImportJsonFile', () => {
  it('imports a workflow JSON on the Workflow tab and opens the new canvas', async () => {
    mockDslBridge.inferIsAgentFromImport.mockReturnValue(true);
    createAgent.mockResolvedValue({ code: 0, data: { id: '1' } });

    const { result } = renderHook(() =>
      useHandleImportJsonFile(AgentCategory.AgentCanvas),
    );
    await act(async () => {
      await result.current.onFileUploadOk(uploadPayload({ graph: {} }));
    });

    expect(mockDslBridge.importDsl).toHaveBeenCalled();
    expect(createAgent).toHaveBeenCalledWith({
      title: 'Imported',
      dsl: { graph: {} },
      canvas_category: AgentCategory.AgentCanvas,
    });
    expect(hideFileUploadModal).toHaveBeenCalled();
    expect(mockNavigateToAgent).toHaveBeenCalledWith(
      '1',
      AgentCategory.AgentCanvas,
    );
    expect(mockNavigate).toHaveBeenCalledTimes(1);
  });

  it('imports a pipeline JSON on the Pipeline tab and opens the new canvas', async () => {
    mockDslBridge.inferIsAgentFromImport.mockReturnValue(false);
    createAgent.mockResolvedValue({ code: 0, data: { id: '2' } });

    const { result } = renderHook(() =>
      useHandleImportJsonFile(AgentCategory.DataflowCanvas),
    );
    await act(async () => {
      await result.current.onFileUploadOk(uploadPayload({ graph: {} }));
    });

    expect(createAgent).toHaveBeenCalledWith(
      expect.objectContaining({
        canvas_category: AgentCategory.DataflowCanvas,
      }),
    );
    expect(mockNavigateToAgent).toHaveBeenCalledWith(
      '2',
      AgentCategory.DataflowCanvas,
    );
    expect(mockNavigate).toHaveBeenCalledTimes(1);
  });

  it('stays on the list when creation fails', async () => {
    mockDslBridge.inferIsAgentFromImport.mockReturnValue(true);
    createAgent.mockResolvedValue({ code: 1 });

    const { result } = renderHook(() =>
      useHandleImportJsonFile(AgentCategory.AgentCanvas),
    );
    await act(async () => {
      await result.current.onFileUploadOk(uploadPayload({ graph: {} }));
    });

    expect(hideFileUploadModal).not.toHaveBeenCalled();
    expect(mockNavigateToAgent).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it.each([
    ['pipeline file on the Workflow tab', false, AgentCategory.AgentCanvas],
    ['workflow file on the Pipeline tab', true, AgentCategory.DataflowCanvas],
  ])(
    'rejects a %s without converting, creating or navigating',
    async (_label, isAgent, expectedCategory) => {
      mockDslBridge.inferIsAgentFromImport.mockReturnValue(isAgent);

      const { result } = renderHook(() =>
        useHandleImportJsonFile(expectedCategory),
      );
      await act(async () => {
        await result.current.onFileUploadOk(uploadPayload({ graph: {} }));
      });

      expect(mockMessage.error).toHaveBeenCalledWith(
        'flow.jsonUploadCategoryMismatch',
      );
      expect(mockDslBridge.importDsl).not.toHaveBeenCalled();
      expect(createAgent).not.toHaveBeenCalled();
      expect(mockNavigateToAgent).not.toHaveBeenCalled();
    },
  );

  it('rejects empty JSON content as before', async () => {
    const { result } = renderHook(() =>
      useHandleImportJsonFile(AgentCategory.AgentCanvas),
    );
    await act(async () => {
      await result.current.onFileUploadOk(uploadPayload({}));
    });

    expect(mockMessage.error).toHaveBeenCalledWith(
      'flow.jsonUploadContentErrorMessage',
    );
    expect(createAgent).not.toHaveBeenCalled();
    expect(mockNavigateToAgent).not.toHaveBeenCalled();
  });
});
