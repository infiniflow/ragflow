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
import { fireEvent, render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { ChildrenDelimiterForm } from './children-delimiter-form';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

type FormValues = {
  parser_config: {
    enable_children: boolean;
    children_delimiter: string;
  };
};

function Harness() {
  const form = useForm<FormValues>({
    defaultValues: {
      parser_config: {
        enable_children: false,
        children_delimiter: '',
      },
    },
  });

  return (
    <TooltipProvider>
      <Form {...form}>
        <ChildrenDelimiterForm />
      </Form>
    </TooltipProvider>
  );
}

describe('ChildrenDelimiterForm', () => {
  it('updates delimiter input visibility when the children switch changes', () => {
    render(<Harness />);

    expect(
      screen.queryByTestId('ds-settings-parser-child-chunk-delimiter-input'),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('switch'));
    expect(
      screen.getByTestId('ds-settings-parser-child-chunk-delimiter-input'),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole('switch'));
    expect(
      screen.queryByTestId('ds-settings-parser-child-chunk-delimiter-input'),
    ).not.toBeInTheDocument();
  });
});
