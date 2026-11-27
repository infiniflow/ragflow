import { render, screen } from '@testing-library/react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { ResourceMetadata } from './resource-metadata';

describe('ResourceMetadata', () => {
  it('renders creation time and additional metadata with secondary text styles', () => {
    const { container } = render(
      <ResourceMetadata
        createTime="14/03/2025 14:53:39"
        items={[
          { key: 'size', content: '1.5 GB' },
          { key: 'pending', content: '2 files pending' },
          { key: 'parsing', content: '12 files parsing' },
        ]}
      />,
    );

    expect(screen.getByText(/common.createdAt/)).toHaveTextContent(
      'common.createdAt 14/03/2025 14:53:39',
    );
    expect(screen.getByText('1.5 GB')).toBeInTheDocument();
    expect(screen.getByText('2 files pending')).toBeInTheDocument();
    expect(screen.getByText('12 files parsing')).toBeInTheDocument();
    expect(container.firstChild).toHaveClass(
      'min-w-0',
      'truncate',
      'text-xs',
      'text-text-secondary',
    );
  });

  it('renders items when creation time is unavailable', () => {
    render(<ResourceMetadata items={[{ key: 'size', content: '0 bytes' }]} />);

    expect(screen.getByText('0 bytes')).toBeInTheDocument();
    expect(screen.queryByText(/common.createdAt/)).not.toBeInTheDocument();
  });

  it('renders nothing without creation time or items', () => {
    const { container } = render(<ResourceMetadata />);

    expect(container).toBeEmptyDOMElement();
  });
});
