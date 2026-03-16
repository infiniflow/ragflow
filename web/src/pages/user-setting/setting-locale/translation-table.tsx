import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react';
import { useMemo, useState } from 'react';

type TranslationTableRow = {
  key: string;
  [language: string]: string;
};

interface TranslationTableProps {
  data: TranslationTableRow[];
  languages: string[];
}

type FilterType = 'all' | 'show_empty' | 'show_non_empty';
type SortOrder = 'asc' | 'desc' | null;

interface ColumnState {
  key: string;
  sortOrder: SortOrder;
  filter: FilterType;
}

const TranslationTable: React.FC<TranslationTableProps> = ({
  data,
  languages,
}) => {
  const [columnStates, setColumnStates] = useState<ColumnState[]>(
    [{ key: 'key', sortOrder: null, filter: 'all' as FilterType }].concat(
      languages.map((lang) => ({
        key: lang,
        sortOrder: null,
        filter: 'all' as FilterType,
      })),
    ),
  );

  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // Get the active sort column
  const activeSortColumn = useMemo(() => {
    return columnStates.find((col) => col.sortOrder !== null);
  }, [columnStates]);

  // Apply sorting and filtering
  const processedData = useMemo(() => {
    let filtered = [...data];

    // Apply filters for all columns
    columnStates.forEach((colState) => {
      if (colState.filter !== 'all') {
        filtered = filtered.filter((record) => {
          const value = record[colState.key];
          if (colState.filter === 'show_empty') {
            return !value || value.length === 0;
          }
          if (colState.filter === 'show_non_empty') {
            return value && value.length > 0;
          }
          return true;
        });
      }
    });

    // Apply sorting
    if (activeSortColumn && activeSortColumn.sortOrder) {
      filtered.sort((a, b) => {
        const aValue = a[activeSortColumn.key] || '';
        const bValue = b[activeSortColumn.key] || '';
        const comparison = String(aValue).localeCompare(String(bValue));
        return activeSortColumn.sortOrder === 'asc' ? comparison : -comparison;
      });
    }

    return filtered;
  }, [data, columnStates, activeSortColumn]);

  // Apply pagination
  const paginatedData = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    const end = start + pageSize;
    return processedData.slice(start, end);
  }, [processedData, currentPage, pageSize]);

  const handleSort = (columnKey: string) => {
    setColumnStates((prev) =>
      prev.map((col) => {
        if (col.key === columnKey) {
          let newOrder: SortOrder = 'asc';
          if (col.sortOrder === 'asc') {
            newOrder = 'desc';
          } else if (col.sortOrder === 'desc') {
            newOrder = null;
          }
          return { ...col, sortOrder: newOrder };
        }
        return { ...col, sortOrder: null };
      }),
    );
  };

  const handleFilter = (columnKey: string, filter: FilterType) => {
    setColumnStates((prev) =>
      prev.map((col) => (col.key === columnKey ? { ...col, filter } : col)),
    );
    setCurrentPage(1);
  };

  const renderSortIcon = (columnKey: string) => {
    const colState = columnStates.find((col) => col.key === columnKey);
    const sortOrder = colState?.sortOrder;

    if (sortOrder === 'asc') {
      return <ArrowUp className="ml-1 h-4 w-4" />;
    } else if (sortOrder === 'desc') {
      return <ArrowDown className="ml-1 h-4 w-4" />;
    } else {
      return <ArrowUpDown className="ml-1 h-4 w-4" />;
    }
  };

  const handlePageChange = (page: number, size: number) => {
    setCurrentPage(page);
    setPageSize(size);
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-lg bg-bg-input scrollbar-auto overflow-hidden border border-border-default">
        <Table rootClassName="rounded-lg">
          <TableHeader className="bg-bg-title">
            <TableRow className="hover:bg-bg-title">
              <TableHead
                className="h-12 px-4 cursor-pointer sticky left-0 bg-bg-title"
                onClick={() => handleSort('key')}
              >
                <div className="flex items-center min-w-[200px]">
                  Key
                  {renderSortIcon('key')}
                </div>
              </TableHead>
              {languages.map((lang) => {
                const colState = columnStates.find((col) => col.key === lang)!;
                return (
                  <TableHead key={lang} className="h-12 px-4">
                    <div className="flex flex-col gap-2">
                      <div
                        className="flex items-center cursor-pointer"
                        onClick={() => handleSort(lang)}
                      >
                        {lang}
                        {renderSortIcon(lang)}
                      </div>
                      <div className="flex gap-1">
                        <button
                          className={`text-xs px-2 py-0.5 rounded ${
                            colState.filter === 'show_empty'
                              ? 'bg-bg-card text-text-primary'
                              : 'bg-bg-title text-text-secondary hover:bg-bg-card'
                          }`}
                          onClick={() => handleFilter(lang, 'show_empty')}
                        >
                          Empty
                        </button>
                        <button
                          className={`text-xs px-2 py-0.5 rounded ${
                            colState.filter === 'show_non_empty'
                              ? 'bg-bg-card text-text-primary'
                              : 'bg-bg-title text-text-secondary hover:bg-bg-card'
                          }`}
                          onClick={() => handleFilter(lang, 'show_non_empty')}
                        >
                          Non-Empty
                        </button>
                        <button
                          className={`text-xs px-2 py-0.5 rounded ${
                            colState.filter === 'all'
                              ? 'bg-bg-card text-text-primary'
                              : 'bg-bg-title text-text-secondary hover:bg-bg-card'
                          }`}
                          onClick={() => handleFilter(lang, 'all')}
                        >
                          All
                        </button>
                      </div>
                    </div>
                  </TableHead>
                );
              })}
            </TableRow>
          </TableHeader>
          <TableBody className="bg-bg-base">
            {paginatedData.length > 0 ? (
              paginatedData.map((record) => (
                <TableRow key={record.key} className="hover:bg-bg-card">
                  <TableCell className="p-4 font-medium sticky left-0 bg-bg-base hover:bg-bg-card">
                    {record.key}
                  </TableCell>
                  {languages.map((lang) => (
                    <TableCell key={lang} className="p-4">
                      {record[lang] || ''}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={languages.length + 1}
                  className="h-24 text-center"
                >
                  No data
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <RAGFlowPagination
        current={currentPage}
        pageSize={pageSize}
        total={processedData.length}
        onChange={handlePageChange}
      />
    </div>
  );
};

export default TranslationTable;
