import ListFilterBar from '@/components/list-filter-bar';
import { Upload } from 'lucide-react';
import { FilesTable } from './files-table';

export default function Files() {
  return (
    <section className="p-8">
      <ListFilterBar title="Files">
        <Upload />
        Upload file
      </ListFilterBar>
      <FilesTable></FilesTable>
    </section>
  );
}
