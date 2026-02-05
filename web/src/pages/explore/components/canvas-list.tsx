import { SearchInput } from '@/components/ui/input';
import { useFetchAllAgentList } from '@/hooks/use-agent-request';
import { useClientSearch } from '@/hooks/use-client-search';
import { useTranslation } from 'react-i18next';
import { CanvasCard } from './canvas-card';

interface CanvasListProps {
  selectedCanvasId?: string;
  onSelectCanvas: (canvasId: string) => void;
}

export function CanvasList({
  selectedCanvasId,
  onSelectCanvas,
}: CanvasListProps) {
  const { t } = useTranslation();

  const { data: canvasList, loading } = useFetchAllAgentList();

  const { filteredData, handleSearchChange, searchKeyword } = useClientSearch({
    data: canvasList ?? [],
    searchFields: ['title', 'description'],
  });

  return (
    <section className="p-5 flex flex-col h-full">
      <h2 className="text-base font-bold mb-4">{t('explore.canvasList')}</h2>
      <div className="mb-4">
        <SearchInput
          placeholder={t('explore.searchCanvas')}
          onChange={handleSearchChange}
          value={searchKeyword}
        />
      </div>
      <div className="flex-1 overflow-auto space-y-3">
        {filteredData.map((canvas) => (
          <CanvasCard
            key={canvas.id}
            canvas={canvas}
            selected={canvas.id === selectedCanvasId}
            onClick={() => onSelectCanvas(canvas.id)}
          />
        ))}
        {!loading && filteredData.length === 0 && (
          <div className="text-center text-text-secondary py-8">
            {searchKeyword
              ? t('explore.noCanvasFound')
              : t('explore.noCanvasFound')}
          </div>
        )}
      </div>
    </section>
  );
}
