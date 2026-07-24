import { CardContainer } from '@/components/card-container';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  useDeleteCompilationTemplateGroup,
  useFetchCompilationTemplateGroupsByPage,
} from '@/hooks/use-compilation-template-group-request';
import { Routes } from '@/routes';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { CompilationTemplateCard } from './compilation-template-card';

type CompilationOperatorSectionProps = {
  tabs: ReactNode;
};

export function CompilationOperatorSection({
  tabs,
}: CompilationOperatorSectionProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const {
    groups,
    total,
    searchString,
    handleInputChange,
    pagination,
    setPagination,
  } = useFetchCompilationTemplateGroupsByPage();

  const { deleteGroup } = useDeleteCompilationTemplateGroup();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  const handleAdd = useCallback(() => {
    navigate(`${Routes.CompilationTemplatesEditNext}?source=agents`);
  }, [navigate]);

  const handleEdit = useCallback(
    (id: string) => () => {
      navigate(`${Routes.CompilationTemplatesEditNext}/${id}?source=agents`);
    },
    [navigate],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await deleteGroup(id);
    },
    [deleteGroup],
  );

  return (
    <article
      className="size-full min-w-0 flex flex-col"
      data-testid="compilation-operator-list"
    >
      <header className="mb-4 min-w-0 px-5 pt-8">
        <ListFilterBar
          leftPanel={tabs}
          searchString={searchString}
          onSearchChange={handleInputChange}
        >
          <Button onClick={handleAdd} data-testid="create-compilation-template">
            <Plus className="size-[1em]" />
          </Button>
        </ListFilterBar>
      </header>

      {groups.length ? (
        <>
          <CardContainer className="flex-1 overflow-auto px-5">
            {groups.map((item) => (
              <CompilationTemplateCard
                key={item.id}
                data={item}
                onClick={handleEdit(item.id)}
                onDelete={handleDelete}
              />
            ))}
          </CardContainer>

          <footer className="mt-4 px-5 pb-5">
            <RAGFlowPagination
              {...pick(pagination, 'current', 'pageSize')}
              total={total}
              onChange={handlePageChange}
            />
          </footer>
        </>
      ) : (
        <div className="flex-1 flex items-center justify-center text-text-secondary text-sm">
          {t('setting.noTemplates')}
        </div>
      )}
    </article>
  );
}
