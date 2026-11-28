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

import { render, screen } from '@testing-library/react';
import { createRef } from 'react';
import { SettingsFormLayout } from './settings-form-layout';

describe('SettingsFormLayout', () => {
  it('separates the fixed header and actions from the scrollable content', () => {
    const scrollContainerRef = createRef<HTMLDivElement>();

    render(
      <form data-testid="settings-form">
        <SettingsFormLayout
          data-testid="settings-layout"
          aria-label="Settings"
          className="custom-layout"
          header={<div>Settings title</div>}
          actions={<button type="submit">Save</button>}
          scrollContainerRef={scrollContainerRef}
        >
          <div data-testid="settings-content">Form fields</div>
        </SettingsFormLayout>
      </form>,
    );

    const layout = screen.getByTestId('settings-layout');
    const header = screen.getByText('Settings title');
    const content = screen.getByTestId('settings-content');
    const scrollContainer = content.parentElement;
    const saveButton = screen.getByRole('button', { name: 'Save' });
    const footer = saveButton.closest('footer');

    expect(layout).toHaveClass(
      'mx-auto',
      'w-full',
      'max-w-5xl',
      'custom-layout',
    );
    expect(layout).toHaveAttribute('aria-label', 'Settings');
    expect(header.parentElement).toHaveClass('shrink-0', 'px-5');
    expect(scrollContainer).toHaveClass(
      'min-h-0',
      'flex-1',
      'flex-col',
      'overflow-y-auto',
      'scrollbar-auto',
    );
    expect(scrollContainerRef.current).toBe(scrollContainer);
    expect(scrollContainer).not.toContainElement(header);
    expect(footer).toHaveClass('shrink-0', 'p-5');
    expect(scrollContainer).not.toContainElement(saveButton);
    expect(screen.getByTestId('settings-form')).toContainElement(footer);
  });

  it('omits the optional header and footer', () => {
    const { container } = render(
      <SettingsFormLayout>
        <div>Form fields</div>
      </SettingsFormLayout>,
    );

    expect(container.querySelector('footer')).not.toBeInTheDocument();
    expect(screen.getByText('Form fields').parentElement).toHaveClass(
      'overflow-y-auto',
    );
  });
});
