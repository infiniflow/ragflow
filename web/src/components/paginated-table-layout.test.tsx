import { render, screen } from '@testing-library/react';
import { PaginatedTableLayout } from './paginated-table-layout';

function TableContent() {
  return (
    <tbody>
      <tr>
        <td>Table content</td>
      </tr>
    </tbody>
  );
}

describe('PaginatedTableLayout', () => {
  it('renders footer metadata before pagination and preserves table props', () => {
    const { container } = render(
      <PaginatedTableLayout
        tableClassName="table-fixed"
        footerLeading={<span>Resource metadata</span>}
        pagination={<button>Pagination</button>}
      >
        <TableContent />
      </PaginatedTableLayout>,
    );

    expect(screen.getByText('Table content')).toBeInTheDocument();
    expect(container.querySelector('table')).toHaveClass('table-fixed');

    const pagination = screen.getByRole('button', { name: 'Pagination' });
    const footer = pagination.parentElement?.parentElement;
    expect(footer?.firstElementChild).toHaveTextContent('Resource metadata');
    expect(footer?.lastElementChild).toContainElement(pagination);
    expect(pagination.parentElement).toHaveClass('ml-auto', 'shrink-0');
  });

  it('keeps pagination right aligned without footer metadata', () => {
    render(
      <PaginatedTableLayout pagination={<button>Pagination</button>}>
        <TableContent />
      </PaginatedTableLayout>,
    );

    const pagination = screen.getByRole('button', { name: 'Pagination' });
    const footer = pagination.parentElement?.parentElement;
    expect(footer?.children).toHaveLength(1);
    expect(pagination.parentElement).toHaveClass('ml-auto', 'shrink-0');
  });
});
