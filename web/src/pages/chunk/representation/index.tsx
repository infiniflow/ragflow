import { Button } from '@/components/ui/button';
import { useFetchDocumentStructureGraph } from '@/hooks/use-document-request';
import { Search } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { RepresentationRenderer } from './components/representation-renderer';
import { RepresentationSelect } from './components/representation-select';
import { useSelectedTemplate } from './hooks/use-selected-template';

export default function Representation() {
  const { t } = useTranslation();
  const { data, loading } = useFetchDocumentStructureGraph();
  const templates = data?.templates ?? [];
  const { selectedTemplateId, setSelectedTemplateId, selectedTemplate } =
    useSelectedTemplate(templates);

  const handleSearch = useCallback(() => {
    // TODO: implement search or refetch
  }, []);

  return (
    <section className="p-5 rounded-2xl h-full flex flex-col">
      <div className="flex items-center justify-between">
        <RepresentationSelect
          templates={templates}
          value={selectedTemplateId}
          onChange={setSelectedTemplateId}
        />
        <Button
          variant="ghost"
          size="icon"
          type="button"
          onClick={handleSearch}
          aria-label={t('chunk.search', 'Search')}
        >
          <Search className="h-5 w-5" />
        </Button>
      </div>
      {loading && (
        <div className="mt-6 text-text-secondary">
          {t('common.loading', 'Loading...')}
        </div>
      )}
      {!loading && templates.length === 0 && (
        <div className="mt-6 text-text-secondary">
          {t(
            'chunk.representationEmpty',
            'No representation templates available.',
          )}
        </div>
      )}
      {!loading && templates.length > 0 && (
        <RepresentationRenderer template={selectedTemplate} />
      )}
    </section>
  );
}
