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

import { buttonVariants } from '@/components/ui/button';
import { APIMapUrl, LLMFactory } from '@/constants/llm';

import {
  AIMLAPI_KEYS_URL,
  AimlapiGetKeyButton,
} from './aimlapi-get-key-button';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

describe('AimlapiGetKeyButton', () => {
  it('links to the AI/ML API keys page', () => {
    render(<AimlapiGetKeyButton />);

    const link = screen.getByRole('link', { name: /aimlapiGetKey/ });
    expect(AIMLAPI_KEYS_URL).toBe('https://aimlapi.com/app/keys');
    expect(link).toHaveAttribute('href', 'https://aimlapi.com/app/keys');
  });

  it('uses the same URL as the rest of the provider settings', () => {
    render(<AimlapiGetKeyButton />);

    // provider-header-bar and available-models link through APIMapUrl; the
    // button must not drift to a second address
    expect(screen.getByRole('link', { name: /aimlapiGetKey/ })).toHaveAttribute(
      'href',
      APIMapUrl[LLMFactory.AIMLAPI],
    );
  });

  it('opens it in a new tab without handing over the opener', () => {
    render(<AimlapiGetKeyButton />);

    const link = screen.getByRole('link', { name: /aimlapiGetKey/ });
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('looks like the secondary button it replaced', () => {
    render(<AimlapiGetKeyButton />);

    const link = screen.getByRole('link', { name: /aimlapiGetKey/ });
    expect(link).toHaveClass(
      ...buttonVariants({ variant: 'secondary', size: 'sm' }).split(/\s+/),
    );
  });

  it('starts no in-app authorization flow', () => {
    // The device-authorization endpoints it used to call have no handler in
    // the Go backend; the control must not be a button that posts anywhere.
    render(<AimlapiGetKeyButton />);

    expect(screen.queryByRole('button')).toBeNull();
  });
});
