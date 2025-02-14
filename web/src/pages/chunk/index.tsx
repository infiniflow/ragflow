import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import {
  QueryStringMap,
  useNavigatePage,
} from '@/hooks/logic-hooks/navigate-hooks';
import { Routes } from '@/routes';
import { EllipsisVertical } from 'lucide-react';
import { useMemo } from 'react';
import { Outlet, useLocation } from 'umi';

export default function ChunkPage() {
  const { navigateToDataset, getQueryString, navigateToChunk } =
    useNavigatePage();
  const location = useLocation();

  const options = useMemo(() => {
    return [
      {
        label: 'Parsed results',
        value: Routes.ParsedResult,
      },
      {
        label: 'Chunk result',
        value: Routes.ChunkResult,
      },
      {
        label: 'Result view',
        value: Routes.ResultView,
      },
    ];
  }, []);

  const path = useMemo(() => {
    return location.pathname.split('/').slice(0, 3).join('/');
  }, [location.pathname]);

  return (
    <section>
      <PageHeader
        title="Editing block"
        back={navigateToDataset(
          getQueryString(QueryStringMap.KnowledgeId) as string,
        )}
      >
        <div>
          <Segmented
            options={options}
            value={path}
            onChange={navigateToChunk as (val: SegmentedValue) => void}
            className="bg-colors-background-inverse-standard text-colors-text-neutral-standard"
          ></Segmented>
        </div>
        <div className="flex items-center gap-2">
          <Button variant={'icon'} size={'icon'}>
            <EllipsisVertical />
          </Button>
          <Button variant={'tertiary'} size={'sm'}>
            Save
          </Button>
        </div>
      </PageHeader>
      <Outlet />
    </section>
  );
}
