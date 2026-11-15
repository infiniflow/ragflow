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

import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { CompilationTemplateFormField } from './compilation-template-form-field';

// jsdom does not provide ResizeObserver or scrollIntoView, which cmdk
// (inside SelectWithSearch's popover) relies on.
beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.scrollIntoView = () => {};
});

const mockUseCompilationTemplateGroupOptions = jest.fn();
const mockCreateDialog = jest.fn<null, [props: unknown]>(() => null);

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

jest.mock('@/hooks/use-compilation-template-group-request', () => ({
  useCompilationTemplateGroupOptions: (...args: unknown[]) =>
    mockUseCompilationTemplateGroupOptions(...args),
}));

jest.mock('./create-compilation-template-group-dialog', () => ({
  CreateCompilationTemplateGroupDialog: (props: unknown) =>
    mockCreateDialog(props),
}));

type DialogProps = {
  open: boolean;
  onCreated: (group: { id: string }) => void;
};

const lastDialogProps = (): DialogProps =>
  mockCreateDialog.mock.calls.at(-1)?.[0] as DialogProps;

let capturedForm: { getValues: (name: string) => unknown };

function Harness({
  groupId,
  ownerTenantId,
}: {
  groupId: string;
  ownerTenantId?: string;
}) {
  const form = useForm({
    defaultValues: { compilation_template_group_id: groupId },
  });
  capturedForm = form;

  return (
    <TooltipProvider>
      <Form {...form}>
        <CompilationTemplateFormField
          name="compilation_template_group_id"
          ownerTenantId={ownerTenantId}
        />
      </Form>
    </TooltipProvider>
  );
}

describe('CompilationTemplateFormField', () => {
  beforeEach(() => {
    mockCreateDialog.mockClear();
    mockUseCompilationTemplateGroupOptions.mockReturnValue({
      options: [{ label: 'Group A', value: 'g1' }],
      isFetched: true,
      isError: false,
    });
  });

  it('shows a warning marker for a group the current user cannot resolve', () => {
    render(<Harness groupId="gone" />);

    const trigger = screen.getByRole('combobox');
    expect(within(trigger).getByText('gone')).toBeInTheDocument();
    expect(trigger.querySelector('svg.size-4')).not.toBeNull();
  });

  it('suppresses the warning while the group list is loading', () => {
    mockUseCompilationTemplateGroupOptions.mockReturnValue({
      options: [],
      isFetched: false,
      isError: false,
    });

    render(<Harness groupId="gone" />);

    const trigger = screen.getByRole('combobox');
    expect(within(trigger).getByText('gone')).toBeInTheDocument();
    expect(trigger.querySelector('svg.size-4')).toBeNull();
  });

  it('shows the option label for a resolvable group', () => {
    render(<Harness groupId="g1" />);

    const trigger = screen.getByRole('combobox');
    expect(within(trigger).getByText('Group A')).toBeInTheDocument();
    expect(trigger.querySelector('svg.size-4')).toBeNull();
  });

  it('requests the options of the canvas owner tenant', () => {
    render(<Harness groupId="g1" ownerTenantId="owner-1" />);

    expect(mockUseCompilationTemplateGroupOptions).toHaveBeenCalledWith(
      'owner-1',
    );
  });

  it('opens the create dialog from the label link', () => {
    render(<Harness groupId="g1" />);

    fireEvent.click(screen.getByText('knowledgeConfiguration.createTemplate'));

    expect(lastDialogProps().open).toBe(true);
  });

  it('adopts the created group when nothing is selected', async () => {
    render(<Harness groupId="" />);

    await act(async () => {
      lastDialogProps().onCreated({ id: 'new-group' });
    });

    expect(capturedForm.getValues('compilation_template_group_id')).toBe(
      'new-group',
    );
  });

  it('keeps the current selection when one already exists', async () => {
    render(<Harness groupId="g1" />);

    await act(async () => {
      lastDialogProps().onCreated({ id: 'new-group' });
    });

    expect(capturedForm.getValues('compilation_template_group_id')).toBe('g1');
  });
});
