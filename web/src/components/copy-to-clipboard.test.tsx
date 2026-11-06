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

import { fireEvent, render, screen } from '@testing-library/react';
import CopyToClipboard from './copy-to-clipboard';

let mockCopyResult = false;

jest.mock('react-copy-to-clipboard', () => ({
  CopyToClipboard: ({
    text,
    onCopy,
    children,
  }: {
    text: string;
    onCopy: (text: string, result: boolean) => void;
    children: JSX.Element;
  }) => <div onClick={() => onCopy(text, mockCopyResult)}>{children}</div>,
}));

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

jest.mock('./ui/tooltip', () => ({
  Tooltip: ({
    children,
    open,
  }: {
    children: JSX.Element[];
    open?: boolean;
  }) => (
    <div data-testid="tooltip" data-open={String(Boolean(open))}>
      {children}
    </div>
  ),
  TooltipContent: ({ children }: { children: JSX.Element }) => (
    <div>{children}</div>
  ),
  TooltipTrigger: ({ children }: { children: JSX.Element }) => <>{children}</>,
}));

describe('CopyToClipboard', () => {
  beforeEach(() => {
    mockCopyResult = false;
  });

  it('does not show success when copying fails', () => {
    render(<CopyToClipboard text="text" avoidButtonWrapper />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    expect(
      button.querySelector('[aria-label="copied"]'),
    ).not.toBeInTheDocument();
    expect(button.querySelector('[aria-label="copy"]')).toBeInTheDocument();
    expect(screen.getByTestId('tooltip')).toHaveAttribute('data-open', 'false');
  });

  it('shows success when copying succeeds', () => {
    mockCopyResult = true;
    render(<CopyToClipboard text="text" avoidButtonWrapper />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    expect(button.querySelector('[aria-label="copied"]')).toBeInTheDocument();
    expect(screen.getByTestId('tooltip')).toHaveAttribute('data-open', 'true');
  });
});
