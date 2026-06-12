import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  useDeleteCompilationTemplate,
  useListCompilationTemplates,
} from '@/hooks/use-compilation-template-request';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProfileSettingWrapperCard } from '../components/user-setting-header';
import { EditTemplateDialog } from './edit-template-dialog';
import { TemplateCardList } from './template-card';

export default function KnowledgeCompilation() {
  const { t } = useTranslation();
  const { data, setPagination, searchString, handleInputChange, pagination } =
    useListCompilationTemplates();
  const { deleteCompilationTemplate } = useDeleteCompilationTemplate();

  const [editVisible, setEditVisible] = useState(false);
  const [editingId, setEditingId] = useState<string>('');

  const showEdit = useCallback(
    (id: string) => () => {
      setEditingId(id);
      setEditVisible(true);
    },
    [],
  );

  const hideEdit = useCallback(() => {
    setEditVisible(false);
    setEditingId('');
  }, []);

  const handleEditFromCard = useCallback((id: string) => {
    setEditingId(id);
    setEditVisible(true);
  }, []);

  const handleDelete = useCallback(
    (id: string) => {
      deleteCompilationTemplate([id]);
    },
    [deleteCompilationTemplate],
  );

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  return (
    <ProfileSettingWrapperCard
      header={
        <header className="flex flex-row gap-1.5 justify-between items-end">
          <div>
            <h2 className="text-text-primary text-2xl font-medium">
              {t('knowledgeCompilation.title')}
            </h2>
            <p className="mt-1 text-text-secondary text-sm">
              {t('knowledgeCompilation.subtitle')}
            </p>
          </div>
          <div className="flex gap-4" role="toolbar">
            <SearchInput
              className="w-40"
              value={searchString}
              onChange={handleInputChange}
              placeholder={t('common.search')}
            />
            <Button onClick={showEdit('')}>
              <Plus /> {t('knowledgeCompilation.addTemplate')}
            </Button>
          </div>
        </header>
      }
    >
      <div className="h-full p-5 overflow-x-hidden overflow-y-auto">
        {data.templates?.length ? (
          <>
            <TemplateCardList
              items={data.templates}
              onEdit={handleEditFromCard}
              onDelete={handleDelete}
            />
            <div className="mt-8">
              <RAGFlowPagination
                {...pick(pagination, 'current', 'pageSize')}
                total={pagination.total || 0}
                onChange={handlePageChange}
              />
            </div>
          </>
        ) : (
          <div className="flex items-center border border-dashed border-border-button rounded-md p-10 w-[590px]">
            <div className="text-text-secondary text-sm">
              {t('knowledgeCompilation.empty')}
            </div>
          </div>
        )}
      </div>

      {editVisible && (
        <EditTemplateDialog hideModal={hideEdit} id={editingId} />
      )}
    </ProfileSettingWrapperCard>
  );
}
