import ListFilterBar from '@/components/list-filter-bar';
import { Upload } from 'lucide-react';
import { DatasetTable } from './dataset-table';

export default function Dataset() {
  return (
    <section className="p-8 text-foreground">
      <ListFilterBar title="Files">
        <Upload />
        Upload file
      </ListFilterBar>
      <DatasetTable></DatasetTable>
    </section>
  );
}
