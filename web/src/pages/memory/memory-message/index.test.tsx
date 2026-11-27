import { render, screen } from '@testing-library/react';
import { formatDate } from '@/utils/date';

const mockMemory = {
  create_time: 1741964019000,
  operator_permission: 0,
  tenant_id: 'tenant-1',
};

jest.mock('@/components/list-filter-bar', () => ({
  __esModule: true,
  default: () => <div>List filter</div>,
}));

jest.mock('@/ee/pages/memories/memory-dropdown-guard', () => ({
  useCanWriteMemoryRole: () => true,
}));

jest.mock('../hooks/use-memory-setting', () => ({
  useFetchMemoryBaseConfiguration: () => ({ data: mockMemory }),
}));

jest.mock('./hook', () => ({
  useFetchMemoryMessageList: () => ({
    searchString: '',
    data: { messages: { message_list: [], total_count: 0 } },
    pagination: { current: 1, pageSize: 10, total: 0 },
    handleInputChange: jest.fn(),
    setPagination: jest.fn(),
  }),
}));

jest.mock('./message-table', () => ({
  MemoryTable: ({ createTime }: { createTime?: string }) => (
    <div data-testid="memory-create-time">{createTime}</div>
  ),
}));

import MemoryMessage from './index';

describe('MemoryMessage', () => {
  it('formats the selected memory creation time like the memory list card', () => {
    render(<MemoryMessage />);

    expect(screen.getByTestId('memory-create-time')).toHaveTextContent(
      formatDate(mockMemory.create_time),
    );
  });
});
