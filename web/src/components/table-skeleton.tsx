import { PropsWithChildren } from 'react';
import { SkeletonCard } from './skeleton-card';
import { TableCell, TableRow } from './ui/table';

type IProps = { columnsLength: number };

function Row({ children, columnsLength }: PropsWithChildren & IProps) {
  return (
    <TableRow>
      <TableCell colSpan={columnsLength} className="h-24 text-center ">
        {children}
      </TableCell>
    </TableRow>
  );
}

export function TableSkeleton({ columnsLength }: { columnsLength: number }) {
  return (
    <Row columnsLength={columnsLength}>
      <SkeletonCard></SkeletonCard>
    </Row>
  );
}

export function TableEmpty({ columnsLength }: { columnsLength: number }) {
  return <Row columnsLength={columnsLength}>No results.</Row>;
}
