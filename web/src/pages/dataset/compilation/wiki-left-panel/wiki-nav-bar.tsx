import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { IArtifact } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { CreateDirectoryDialog } from '../create-directory-dialog';
import { useCreateDirectory } from '../hooks/use-create-directory';
import { useWikiNavigation } from './hooks/use-wiki-navigation';
import { WikiArtifactList } from './wiki-artifact-list';
import { WikiTopicList } from './wiki-topic-list';

type WikiNavBarProps = {
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
};

export function WikiNavBar({
  selectedArtifact,
  onSelectArtifact,
}: WikiNavBarProps) {
  const { t } = useTranslation();
  const {
    scrollRef,
    searchString,
    selectedTopic,
    topics,
    artifacts,
    loading,
    hasMore,
    handleSearchChange,
    handleSelectTopic,
    handleBackToTopics,
    handleScroll,
  } = useWikiNavigation();
  const {
    open,
    loading: createLoading,
    form,
    showModal: handleShowCreateDialog,
    hideModal: handleHideCreateDialog,
    handleOk: handleCreateOk,
  } = useCreateDirectory();

  return (
    <div className="size-full flex flex-col gap-3 px-3">
      <SearchInput
        placeholder={t('common.search')}
        value={searchString}
        onChange={handleSearchChange}
      />
      <section>
        <div className="flex items-center justify-between">
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink
                  onClick={handleBackToTopics}
                  className={cn(
                    'text-sm',
                    selectedTopic
                      ? 'text-text-secondary cursor-pointer'
                      : 'text-text-primary cursor-default',
                  )}
                >
                  {t('knowledgeDetails.topics')}
                </BreadcrumbLink>
              </BreadcrumbItem>
              {selectedTopic && (
                <>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbLink className="text-text-primary text-sm cursor-default">
                      {selectedTopic.title}
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                </>
              )}
            </BreadcrumbList>
          </Breadcrumb>
          {!selectedTopic && (
            <Button
              variant="secondary"
              size="icon-xs"
              onClick={handleShowCreateDialog}
            >
              <Plus className="size-4" />
            </Button>
          )}
        </div>
      </section>
      <CreateDirectoryDialog
        open={open}
        loading={createLoading}
        form={form}
        onOk={handleCreateOk}
        onCancel={handleHideCreateDialog}
      />
      <div
        ref={scrollRef}
        className="flex-1 min-h-0 overflow-y-auto pb-3"
        onScroll={handleScroll}
      >
        {selectedTopic ? (
          <WikiArtifactList
            artifacts={artifacts}
            selectedArtifact={selectedArtifact}
            loading={loading}
            hasMore={hasMore}
            onSelectArtifact={onSelectArtifact}
          />
        ) : (
          <WikiTopicList
            topics={topics}
            loading={loading}
            hasMore={hasMore}
            onSelectTopic={handleSelectTopic}
          />
        )}
      </div>
    </div>
  );
}
