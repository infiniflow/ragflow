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

import { ACTION_CONTRACT_FIELD_ORDER } from '@/interfaces/database/compilation-model';

import { ActionContractBlock } from './action-contract-block';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, options?: { defaultValue?: string }) =>
      options?.defaultValue ?? _key,
  }),
}));

// `any` for the fixtures: this file contains jest.mock, so it goes through
// esbuild-jest's babel pass, which cannot strip imported types used as
// annotations (see canvas-editor.test.tsx).
/** All six fields, with the named ones carrying a value. */
const contractFields = (filled: any) => {
  const fields: any = {};
  ACTION_CONTRACT_FIELD_ORDER.forEach((name) => {
    const value = filled[name];
    const status = value === undefined ? 'missing' : 'present';
    fields[name] = {
      status,
      value,
      provenance: status === 'missing' ? 'absent' : 'document',
    };
  });
  return fields;
};

describe('ActionContractBlock', () => {
  // A template may narrow the contract, and the row records which fields it was
  // asked for. Judging it against the fixed six would show every action of that
  // model as permanently incomplete — and teach the reviewer to ignore the list.
  it('judges a narrowed contract against its declaration', () => {
    render(
      <ActionContractBlock
        fields={contractFields({ authorization: 'the compliance officer' })}
        missing={['audit_required']}
        declared={1}
        required={['authorization', 'audit_required']}
      />,
    );

    expect(screen.getByText('1/2')).toBeTruthy();
    expect(screen.getByText('authorization')).toBeTruthy();
    expect(screen.getByText('audit_required')).toBeTruthy();
    // Never asked for, so never owed: not rendered as a missing line.
    expect(screen.queryByText('failure_policy')).toBeNull();
    expect(screen.queryByText('preconditions')).toBeNull();
  });

  it('still shows a value outside the declaration', () => {
    // An operator may enter a rule the corpus does not state, and a row compiled
    // before the template narrowed keeps its old values. Hiding either would hide
    // the only place that value can be read.
    render(
      <ActionContractBlock
        fields={contractFields({
          authorization: 'the compliance officer',
          failure_policy: 'escalate to the branch manager',
        })}
        missing={[]}
        declared={1}
        required={['authorization']}
      />,
    );

    expect(screen.getByText('failure_policy')).toBeTruthy();
    expect(screen.getByText('escalate to the branch manager')).toBeTruthy();
    // The denominator stays what was asked for, so the extra row cannot make the
    // contract look unfinished.
    expect(screen.getByText('1/1')).toBeTruthy();
  });

  it('falls back to all six without a declaration', () => {
    render(
      <ActionContractBlock
        fields={contractFields({})}
        missing={[...ACTION_CONTRACT_FIELD_ORDER]}
      />,
    );
    expect(screen.getByText('0/6')).toBeTruthy();
    ACTION_CONTRACT_FIELD_ORDER.forEach((name) => {
      expect(screen.getByText(name)).toBeTruthy();
    });
  });
});
