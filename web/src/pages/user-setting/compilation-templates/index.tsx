import { CardContainer } from '@/components/card-container';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useDeleteCompilationTemplateGroup,
  useFetchCompilationTemplateGroupsByPage,
} from '@/hooks/use-compilation-template-group-request';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { ProfileSettingWrapperCard } from '../components/user-setting-header';
import { TemplateCard } from './template-card';

export default function CompilationTemplates() {
  const { t } = useTranslation();
  const {
    groups,
    total,
    searchString,
    handleInputChange,
    pagination,
    setPagination,
  } = useFetchCompilationTemplateGroupsByPage();

  const { deleteGroup } = useDeleteCompilationTemplateGroup();
  const { navigateToCompilationTemplate } = useNavigatePage();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  const handleAdd = useCallback(() => {
    navigateToCompilationTemplate('create')();
  }, [navigateToCompilationTemplate]);

  const handleEdit = useCallback(
    (id: string) => {
      navigateToCompilationTemplate(id)();
    },
    [navigateToCompilationTemplate],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await deleteGroup(id);
    },
    [deleteGroup],
  );

  return (
    <ProfileSettingWrapperCard
      header={
        <header className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h2 className="text-2xl font-medium text-text-primary">
              {t('setting.compilationTemplates')}
            </h2>

            <p className="mt-1 text-sm text-text-secondary">
              {t('setting.compilationTemplatesDescription')}
            </p>
          </div>

          <div className="flex items-center gap-4">
            <SearchInput
              className="w-52"
              value={searchString}
              onChange={handleInputChange}
              placeholder={t('common.search')}
            />

            <Button onClick={handleAdd}>
              <Plus />
              {t('setting.addTemplateGroup')}
            </Button>
          </div>
        </header>
      }
    >
      <div className="h-full flex flex-col">
        <div className="flex-1 min-h-0 overflow-x-hidden overflow-y-auto p-5">
          {groups.length > 0 ? (
            <CardContainer>
              {groups.map((item) => (
                <TemplateCard
                  key={item.id}
                  data={item}
                  onEdit={handleEdit}
                  onDelete={handleDelete}
                />
              ))}
            </CardContainer>
          ) : (
            <div className="flex items-center justify-center h-full text-text-secondary text-sm">
              {t('setting.noTemplates')}
            </div>
          )}
        </div>

        <div className="p-5 pt-0">
          <RAGFlowPagination
            {...pick(pagination, 'current', 'pageSize')}
            total={total}
            onChange={handlePageChange}
          />
        </div>
      </div>
    </ProfileSettingWrapperCard>
  );
}
