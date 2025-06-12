import { Loader2 } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { TableCell, TableRow } from './ui/table';

type IProps = { columnsLength: number };

function Row({ children, columnsLength }: PropsWithChildren & IProps) {
  return (
    <TableRow>
      <TableCell colSpan={columnsLength} className="h-24 text-center">
        {children}
      </TableCell>
    </TableRow>
  );
}

export function TableSkeleton({
  columnsLength,
  children,
}: PropsWithChildren & IProps) {
  return (
    <Row columnsLength={columnsLength}>
      {children || (
        <Loader2 className="animate-spin size-16 inline-block text-gray-400" />
      )}
    </Row>
  );
}

export function TableEmpty({ columnsLength }: { columnsLength: number }) {
  return <Row columnsLength={columnsLength}>No results.</Row>;
}
