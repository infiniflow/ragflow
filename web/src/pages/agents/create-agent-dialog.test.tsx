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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { CreateAgentDialog } from './create-agent-dialog';
import { FlowType } from './constant';

const mockNavigateToCompilationEditor = jest.fn();
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToCompilationEditor: mockNavigateToCompilationEditor,
  }),
}));

const TitleByType = {
  [FlowType.Agent]: 'Create workflow',
  [FlowType.Flow]: 'Create ingestion pipeline',
  [FlowType.Compiler]: 'Create compilation operator',
};

beforeEach(() => {
  jest.clearAllMocks();
});

describe('CreateAgentDialog', () => {
  it.each([FlowType.Agent, FlowType.Flow, FlowType.Compiler] as const)(
    'shows the %s title and no type selector',
    (selectedType) => {
      render(
        <CreateAgentDialog
          visible
          hideModal={jest.fn()}
          onOk={jest.fn()}
          selectedType={selectedType}
        />,
      );

      expect(screen.getByText(TitleByType[selectedType])).toBeInTheDocument();
      // The canvas type now comes from the active tab; the dialog no longer
      // renders the three-way card selector.
      expect(screen.queryByText('Choose agent type')).toBeNull();
    },
  );

  it('submits the Workflow type when confirmed', async () => {
    const onOk = jest.fn().mockResolvedValue(true);
    render(
      <CreateAgentDialog
        visible
        hideModal={jest.fn()}
        onOk={onOk}
        selectedType={FlowType.Agent}
      />,
    );

    fireEvent.change(screen.getByTestId('agent-name-input'), {
      target: { value: 'My workflow' },
    });
    fireEvent.click(screen.getByTestId('agent-save'));

    await waitFor(() => expect(onOk).toHaveBeenCalled());
    expect(onOk.mock.calls[0][0]).toMatchObject({
      name: 'My workflow',
      type: FlowType.Agent,
    });
  });

  it('submits the Pipeline type when confirmed', async () => {
    const onOk = jest.fn().mockResolvedValue(true);
    render(
      <CreateAgentDialog
        visible
        hideModal={jest.fn()}
        onOk={onOk}
        selectedType={FlowType.Flow}
      />,
    );

    fireEvent.change(screen.getByTestId('agent-name-input'), {
      target: { value: 'My pipeline' },
    });
    fireEvent.click(screen.getByTestId('agent-save'));

    await waitFor(() => expect(onOk).toHaveBeenCalled());
    expect(onOk.mock.calls[0][0]).toMatchObject({
      name: 'My pipeline',
      type: FlowType.Flow,
    });
  });

  it('navigates to the compilation editor instead of creating on Next', () => {
    const onOk = jest.fn();
    render(
      <CreateAgentDialog
        visible
        hideModal={jest.fn()}
        onOk={onOk}
        selectedType={FlowType.Compiler}
      />,
    );

    // Compilation operators are configured on the edit-next page, so the
    // dialog has no name field and no confirm button.
    expect(screen.queryByTestId('agent-name-input')).toBeNull();
    expect(screen.queryByTestId('agent-save')).toBeNull();

    fireEvent.click(screen.getByTestId('agent-next'));
    expect(mockNavigateToCompilationEditor).toHaveBeenCalledWith();
    expect(onOk).not.toHaveBeenCalled();
  });
});
