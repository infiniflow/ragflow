import { TooltipProvider } from '@/components/ui/tooltip';
import type { IChunk } from '@/interfaces/database/dataset';
import { fireEvent, render, screen } from '@testing-library/react';
import { ChunkTextMode } from '../../../constant';
import ChunkCard from '../index';

const ChunkId = '0123456789abcdef0123456789abcdef';

const chunk: IChunk = {
  available_int: 1,
  chunk_id: ChunkId,
  content_with_weight: 'chunk content',
  doc_id: 'doc-1',
  doc_name: 'doc.pdf',
  image_id: '',
  positions: [],
};

function renderChunkCard() {
  return render(
    <TooltipProvider>
      <ChunkCard
        item={chunk}
        checked={false}
        switchChunk={jest.fn()}
        editChunk={jest.fn()}
        handleCheckboxClick={jest.fn()}
        selected={false}
        clickChunkCard={jest.fn()}
        textMode={ChunkTextMode.Full}
      />
    </TooltipProvider>,
  );
}

describe('ChunkCard', () => {
  it('renders the chunk id so it can be matched against retrieval results', () => {
    renderChunkCard();

    expect(screen.getByText(ChunkId)).toBeInTheDocument();
    expect(screen.getByText(ChunkId)).toHaveAttribute('title', ChunkId);
  });

  it('copies the chunk id to the clipboard', () => {
    const execCommand = jest.fn().mockReturnValue(true);
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: execCommand,
    });

    renderChunkCard();

    fireEvent.click(screen.getByLabelText('chunk.copyChunkId'));

    expect(execCommand).toHaveBeenCalledWith('copy');
  });
});
