import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen } from '@testing-library/react';
import { ChunkDetailEntry } from './chunk-detail-entry';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

const ChunkId = '0123456789abcdef0123456789abcdef';

const chunk = {
  available_int: 1,
  chunk_id: ChunkId,
  content_with_weight: 'chunk content',
  doc_id: 'doc-1',
  doc_name: 'doc.pdf',
  image_id: '',
  positions: [],
};

function renderEntry() {
  return render(
    <TooltipProvider>
      <ChunkDetailEntry chunk={chunk} />
    </TooltipProvider>,
  );
}

describe('ChunkDetailEntry', () => {
  it('renders the chunk id so it can be matched against retrieval results', () => {
    renderEntry();

    expect(screen.getByText(ChunkId)).toHaveAttribute('title', ChunkId);
  });

  it('copies the chunk id to the clipboard', () => {
    const execCommand = jest.fn().mockReturnValue(true);
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: execCommand,
    });

    renderEntry();

    fireEvent.click(screen.getByLabelText('chunk.copyChunkId'));

    expect(execCommand).toHaveBeenCalledWith('copy');
  });
});
