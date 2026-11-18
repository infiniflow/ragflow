import { render, screen } from '@testing-library/react';

import { MessageVirtualizer } from '@/hooks/use-virtual-messages';
import { VirtualMessageList } from './index';

function buildVirtualizer() {
  return {
    getTotalSize: () => 300,
    getVirtualItems: () => [
      { index: 0, key: 'user_1', start: 0, size: 100, end: 100, lane: 0 },
      { index: 2, key: 'user_3', start: 200, size: 100, end: 300, lane: 0 },
    ],
    measureElement: jest.fn(),
  } as unknown as MessageVirtualizer;
}

describe('VirtualMessageList', () => {
  it('renders only the virtual items the virtualizer returns', () => {
    render(
      <VirtualMessageList
        items={['one', 'two', 'three']}
        virtualizer={buildVirtualizer()}
        renderItem={(item, index) => <span>{`${index}:${item}`}</span>}
      />,
    );

    expect(screen.getByText('0:one')).toBeInTheDocument();
    expect(screen.getByText('2:three')).toBeInTheDocument();
    expect(screen.queryByText('1:two')).not.toBeInTheDocument();
  });

  it('positions each item at the offset reported by the virtualizer', () => {
    render(
      <VirtualMessageList
        items={['one', 'two', 'three']}
        virtualizer={buildVirtualizer()}
        renderItem={(item) => <span>{item}</span>}
      />,
    );

    expect(screen.getByText('one').parentElement).toHaveStyle({
      transform: 'translateY(0px)',
    });
    expect(screen.getByText('three').parentElement).toHaveStyle({
      transform: 'translateY(200px)',
    });
  });
});
