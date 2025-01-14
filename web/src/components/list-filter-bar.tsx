import { Filter, Search } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { Button } from './ui/button';

interface IProps {
  title: string;
  showDialog?: () => void;
}

export default function ListFilterBar({
  title,
  children,
  showDialog,
}: PropsWithChildren<IProps>) {
  return (
    <div className="flex justify-between mb-6">
      <span className="text-3xl font-bold ">{title}</span>
      <div className="flex gap-4 items-center">
        <Filter className="size-5" />
        <Search className="size-5" />
        <Button variant={'tertiary'} size={'sm'} onClick={showDialog}>
          {children}
        </Button>
      </div>
    </div>
  );
}
