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

import { act, fireEvent, render, screen } from '@testing-library/react';
import { ChunkTextMode } from '../../constant';
import ChunkCard from './index';

jest.mock('@/ee/hooks/logic-hooks/use-permission', () => ({
  useDatasetEditButtonDisabled: () => false,
}));

// The history entry is an enterprise seam: it exists in one edition and is a
// no-op in the other, and its real tree reaches the app-wide i18n instance this
// test replaces. The card's own behaviour is what is under test here, so the
// seam is stubbed and the enterprise tree exercises it on its own.
jest.mock(
  '@/ee/pages/chunk/parsed-result/add-knowledge/components/knowledge-chunk/components/chunk-creating-modal/chunk-version-entry',
  () => ({ ChunkVersionEntry: () => null }),
);

// Same seam, same reason: the detail entry reads the version history through
// react-query, which this test renders the card without.
jest.mock('@/ee/pages/chunk/components/chunk-card/chunk-detail-entry', () => ({
  ChunkDetailEntry: () => null,
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

const item = {
  available_int: 0,
  chunk_id: 'chunk-1',
  content_with_weight: 'Chunk content',
  doc_id: 'doc-1',
  doc_name: 'document.pdf',
  image_id: '',
  positions: [],
};

describe('ChunkCard', () => {
  it('disables the switch until its request completes', async () => {
    let resolveSwitch: () => void = () => {};
    const switchChunk = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSwitch = resolve;
        }),
    );

    render(
      <ChunkCard
        item={item}
        checked={false}
        switchChunk={switchChunk}
        editChunk={jest.fn()}
        removeChunk={jest.fn()}
        handleCheckboxClick={jest.fn()}
        selected={false}
        clickChunkCard={jest.fn()}
        textMode={ChunkTextMode.Full}
      />,
    );

    const switchElement = screen.getByRole('switch');
    fireEvent.click(switchElement);

    expect(switchChunk).toHaveBeenCalledTimes(1);
    expect(switchChunk).toHaveBeenCalledWith(1, ['chunk-1']);
    expect(switchElement).toBeDisabled();

    fireEvent.click(switchElement);
    expect(switchChunk).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveSwitch();
    });

    expect(switchElement).toBeEnabled();
  });

  it('leaves the corner strip to the type and the detail entry', () => {
    render(
      <ChunkCard
        item={item}
        checked={false}
        switchChunk={jest.fn()}
        editChunk={jest.fn()}
        removeChunk={jest.fn()}
        handleCheckboxClick={jest.fn()}
        selected={false}
        clickChunkCard={jest.fn()}
        textMode={ChunkTextMode.Full}
      />,
    );

    expect(screen.getByText('chunk.docType.text')).toBeInTheDocument();
    expect(screen.getByText('Chunk content')).toBeInTheDocument();
    // The id and the way into the details belong to the entry, which this test
    // stubs out: an edition without chunk history prints neither the id nor a
    // version of its own.
    expect(screen.queryByText('chunk-1')).not.toBeInTheDocument();
    expect(screen.queryByText(/^v\d+$/)).not.toBeInTheDocument();
  });
});
