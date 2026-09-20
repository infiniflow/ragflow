import {
  useFetchNextChunkList,
  useSwitchChunk,
} from '@/hooks/use-chunk-request';
import type { IChunk } from '@/interfaces/database/dataset';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ChunkCard from './components/chunk-card';
import CreatingModal from './components/chunk-creating-modal';
import { ChunkTextMode } from './constant';
import {
  useChangeChunkTextMode,
  useDeleteChunkByIds,
  useGetChunkHighlights,
  useGetSelectedChunk,
  useHandleChunkCardClick,
  useTargetChunkFromQuery,
  useUpdateChunk,
} from './hooks';

import ChunkResultBar from './components/chunk-result-bar';
import CheckboxSets from './components/chunk-result-bar/checkbox-sets';
import DocumentViewSwitch from './components/document-view-switch';
// import DocumentHeader from './components/document-preview/document-header';

import {
  ClaimsPanel,
  type ClaimsPanelState,
  type EvidencePanelState,
  NodeDetailPanel,
} from '@/pages/chunk/representation/components/claim-list';
import { useGetDocumentUrl } from '@/components/document-preview/hooks';
import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import message from '@/components/ui/message';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { Spin } from '@/components/ui/spin';
import {
  QueryStringMap,
  useNavigatePage,
} from '@/hooks/logic-hooks/navigate-hooks';
import { useClearSelectionOnPageChange } from '@/hooks/logic-hooks/use-clear-selection-on-page-change';
import { getExtension } from '@/utils/document-util';
import { LucideArrowBigLeft } from 'lucide-react';

function Chunk() {
  const { targetChunkId, clearTargetChunkId } = useTargetChunkFromQuery();
  // Arriving from a retrieval-testing hit: show only that chunk, because its
  // position in the full paginated list is unknown.
  const [filterChunkIds, setFilterChunkIds] = useState<string[]>(
    targetChunkId ? [targetChunkId] : [],
  );
  // The route keeps this page mounted when only the query string changes, so
  // history navigation on and off a hit has to move the filter with it.
  useEffect(() => {
    setFilterChunkIds((previousIds) => {
      const nextIds = targetChunkId ? [targetChunkId] : [];
      return previousIds.join() === nextIds.join() ? previousIds : nextIds;
    });
  }, [targetChunkId]);
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  // The artifact tree publishes its claims / evidence content upward; the page
  // renders it as a resizable column between the tree and the chunk list, and
  // shows only two columns while nothing is open.
  const [claimsPanel, setClaimsPanel] = useState<ClaimsPanelState | null>(null);
  const [evidencePanel, setEvidencePanel] = useState<EvidencePanelState | null>(
    null,
  );
  const { removeChunk } = useDeleteChunkByIds();
  const {
    data: { documentInfo, data = [], total },
    pagination,
    loading,
    searchString,
    handleInputChange,
    available,
    handleSetAvailable,
    dataUpdatedAt,
  } = useFetchNextChunkList(true, { chunkIds: filterChunkIds });
  const { handleChunkCardClick, selectedChunkId } =
    useHandleChunkCardClick(targetChunkId);

  const { t } = useTranslation();
  const { changeChunkTextMode, textMode } = useChangeChunkTextMode();
  const { switchChunk } = useSwitchChunk();
  const [chunkList, setChunkList] = useState(data);
  useEffect(() => {
    setChunkList(data);
  }, [data]);
  const {
    chunkUpdatingLoading,
    onChunkUpdatingOk,
    showChunkUpdatingModal,
    hideChunkUpdatingModal,
    chunkId,
    chunkUpdatingVisible,
    documentId,
  } = useUpdateChunk();
  const { navigateToDataFile, getQueryString } = useNavigatePage();
  const fileUrl = useGetDocumentUrl(false);

  const clearSelectedChunkIds = useCallback(() => {
    setSelectedChunkIds([]);
  }, []);

  // Stable identities: the artifact tree republishes its panel content whenever
  // the claims request settles, so an unstable callback would loop that effect.
  const handleClaimsPanelChange = useCallback(
    (panel: ClaimsPanelState | null) => setClaimsPanel(panel),
    [],
  );
  const handleEvidencePanelChange = useCallback(
    (panel: EvidencePanelState | null) => setEvidencePanel(panel),
    [],
  );

  useClearSelectionOnPageChange(pagination, clearSelectedChunkIds);

  const selectAllChunk = useCallback(
    (checked: boolean) => {
      setSelectedChunkIds(checked ? data.map((x) => x.chunk_id) : []);
    },
    [data],
  );

  const handleSingleCheckboxClick = useCallback(
    (chunkId: string, checked: boolean) => {
      setSelectedChunkIds((previousIds) => {
        const idx = previousIds.findIndex((x) => x === chunkId);
        const nextIds = [...previousIds];
        if (checked && idx === -1) {
          nextIds.push(chunkId);
        } else if (!checked && idx !== -1) {
          nextIds.splice(idx, 1);
        }
        return nextIds;
      });
    },
    [],
  );

  const handleChunkIdsChange = useCallback(
    (chunkIds: string[]) => {
      setFilterChunkIds(chunkIds);
      if (chunkIds.length === 0) {
        pagination.onChange?.(1, pagination.pageSize);
      }
    },
    [pagination],
  );

  const showAllChunks = useCallback(() => {
    setFilterChunkIds([]);
    clearTargetChunkId();
  }, [clearTargetChunkId]);

  const showSelectedChunkWarning = useCallback(() => {
    message.warning(t('message.pleaseSelectChunk'));
  }, [t]);

  const handleRemoveChunk = useCallback(async () => {
    if (selectedChunkIds.length > 0) {
      const resCode: number = await removeChunk(selectedChunkIds, documentId);
      if (resCode === 0) {
        clearSelectedChunkIds();
      }
    } else {
      showSelectedChunkWarning();
    }
  }, [
    selectedChunkIds,
    documentId,
    removeChunk,
    showSelectedChunkWarning,
    clearSelectedChunkIds,
  ]);

  const handleSwitchChunk = useCallback(
    async (available?: number, chunkIds?: string[]) => {
      let ids = chunkIds;
      if (!chunkIds) {
        ids = selectedChunkIds;
        if (selectedChunkIds.length === 0) {
          showSelectedChunkWarning();
          return;
        }
      }

      const resCode: number = await switchChunk({
        chunk_ids: ids,
        available_int: available,
        doc_id: documentId,
      });
      if (ids?.length && resCode === 0) {
        chunkList.forEach((x: any) => {
          if (ids.indexOf(x['chunk_id']) > -1) {
            x['available_int'] = available;
          }
        });
        setChunkList(chunkList);
      }
    },
    [
      switchChunk,
      documentId,
      selectedChunkIds,
      showSelectedChunkWarning,
      chunkList,
    ],
  );

  const { highlights, setWidthAndHeight } =
    useGetChunkHighlights(selectedChunkId);
  const selectedChunk = useGetSelectedChunk(selectedChunkId);
  const positions = Array.isArray(selectedChunk?.positions)
    ? selectedChunk.positions
    : [];

  // Two columns until the artifact tree opens a claims / evidence panel: the
  // middle column only exists while there is something to show in it.
  const showArtifactDetail = Boolean(claimsPanel || evidencePanel);

  const fileType = useMemo(() => {
    const name = documentInfo?.name || '';
    if (name.includes('.')) {
      return getExtension(name);
    }
    switch (documentInfo?.type) {
      case 'doc':
      case 'visual':
        return documentInfo?.name?.split('.').pop() || documentInfo.type;
      case 'docx':
      case 'txt':
      case 'md':
      case 'mdx':
      case 'pdf':
        return documentInfo.type;
    }
    return 'unknown';
  }, [documentInfo]);

  // Remount the virtual list exactly when the rendered set changes. Keying on
  // the id sequence (instead of dataUpdatedAt) means a background refetch that
  // returns the same chunks does not reset scroll, while search / page /
  // filter results get a fresh virtualizer with no stale heights.
  const listKey = useMemo(
    () => chunkList.map((x) => x.chunk_id).join(','),
    [chunkList],
  );

  return (
    <main className="h-dvh flex flex-col">
      <PageHeader>
        <Button
          variant="outline"
          onClick={navigateToDataFile(
            getQueryString(QueryStringMap.id) as string,
          )}
        >
          <LucideArrowBigLeft />
          {t('common.back')}
        </Button>
      </PageHeader>

      <Card className="mx-5 mb-5 flex-1 h-0 p-0 bg-transparent shadow-none">
        <CardContent className="p-0 h-full flex flex-row divide-x-0.5 rtl:divide-x-reverse">
          <ResizablePanelGroup direction="horizontal" className="flex-1">
            {/* id + order must be explicit: the middle column mounts after the
                first render, and without them react-resizable-panels orders
                panels by registration, so it would sit AFTER the chunk list
                and its resize handles would drag in the wrong direction. */}
            <ResizablePanel
              id="artifact-tree"
              order={1}
              defaultSize={40}
              minSize={20}
            >
              <article className="h-full flex flex-col">
                <DocumentViewSwitch
                  documentInfo={documentInfo}
                  fileType={fileType}
                  highlights={highlights}
                  setWidthAndHeight={setWidthAndHeight}
                  url={fileUrl}
                  positions={positions}
                  onChunkIdsChange={handleChunkIdsChange}
                  onClaimsPanelChange={handleClaimsPanelChange}
                  onEvidencePanelChange={handleEvidencePanelChange}
                />
              </article>
            </ResizablePanel>

            <ResizableHandle
              withHandle
              className="bg-border-button w-[0.5px]"
            />

            {/* Separate conditionals rather than a fragment: PanelGroup pairs
                each handle with the panels adjacent to it in registration
                order, and a fragment would hide these children from it. */}
            {showArtifactDetail && (
              <ResizablePanel
                id="artifact-detail"
                order={2}
                defaultSize={30}
                minSize={20}
              >
                <article className="h-full flex flex-col">
                  {claimsPanel && (
                    <div className="flex-1 min-h-0">
                      <ClaimsPanel {...claimsPanel} />
                    </div>
                  )}
                  {evidencePanel && (
                    <div className="flex-1 min-h-0">
                      <NodeDetailPanel {...evidencePanel} />
                    </div>
                  )}
                </article>
              </ResizablePanel>
            )}

            {showArtifactDetail && (
              <ResizableHandle
                withHandle
                className="bg-border-button w-[0.5px]"
              />
            )}

            <ResizablePanel
              id="chunk-list"
              order={showArtifactDetail ? 3 : 2}
              defaultSize={60}
              minSize={30}
            >
              <article className="h-full flex flex-col">
                <header className="flex-0 p-5 pb-2.5 border-b-0.5 border-b-border-button">
                  <h2 className="text-[24px]">{t('chunk.chunkResult')}</h2>
                  <div className="text-[14px] text-text-secondary">
                    {t('chunk.chunkResultTip')}
                  </div>
                </header>

                <Spin spinning={loading} className="flex-1 h-0" size="large">
                  <div className="relative @container h-full px-5 pb-5 overflow-hidden flex flex-col">
                    <div
                      className="
                        sticky top-0 z-[1] bg-bg-base space-y-4 py-5
                        @4xl:flex @4xl:justify-between @4xl:items-center
                        @4xl:space-y-0 @4xl:gap-4
                      "
                      role="toolbar"
                    >
                      <ChunkResultBar
                        className="@4xl:order-2"
                        handleInputChange={handleInputChange}
                        searchString={searchString}
                        changeChunkTextMode={changeChunkTextMode}
                        createChunk={showChunkUpdatingModal}
                        available={available}
                        selectAllChunk={selectAllChunk}
                        handleSetAvailable={handleSetAvailable}
                      />

                      <CheckboxSets
                        className="h-8"
                        selectAllChunk={selectAllChunk}
                        switchChunk={handleSwitchChunk}
                        removeChunk={handleRemoveChunk}
                        checked={selectedChunkIds.length === data.length}
                        selectedChunkIds={selectedChunkIds}
                      />
                    </div>

                    {targetChunkId && filterChunkIds.length > 0 && (
                      <div className="mb-4 flex items-center justify-between gap-4 rounded-lg border-0.5 border-border-button bg-bg-card px-4 py-2.5">
                        <span className="text-sm text-text-secondary">
                          {t('chunk.showingRetrievedChunk')}
                        </span>
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={showAllChunks}
                        >
                          {t('chunk.showAllChunks')}
                        </Button>
                      </div>
                    )}

                    <ChunkVirtualList
                      key={listKey}
                      items={chunkList}
                      selectedChunkId={selectedChunkId}
                      selectedChunkIds={selectedChunkIds}
                      textMode={textMode}
                      editChunk={showChunkUpdatingModal}
                      handleCheckboxClick={handleSingleCheckboxClick}
                      switchChunk={handleSwitchChunk}
                      clickChunkCard={handleChunkCardClick}
                      // Changes on every refetch, so a chunk whose image was
                      // replaced in place re-fetches it instead of reusing the
                      // cached bytes.
                      imageCacheKey={dataUpdatedAt}
                    />

                    <footer className="mt-5">
                      <RAGFlowPagination
                        pageSize={pagination.pageSize}
                        current={pagination.current}
                        total={total}
                        onChange={pagination.onChange}
                      />
                    </footer>
                  </div>
                </Spin>
              </article>
            </ResizablePanel>
          </ResizablePanelGroup>
        </CardContent>
      </Card>

      {chunkUpdatingVisible && (
        <CreatingModal
          doc_id={documentId}
          chunkId={chunkId}
          hideModal={hideChunkUpdatingModal}
          visible={chunkUpdatingVisible}
          loading={chunkUpdatingLoading}
          onOk={onChunkUpdatingOk}
          parserId={documentInfo.parser_id}
        />
      )}
    </main>
  );
}

interface ChunkVirtualListProps {
  items: IChunk[];
  selectedChunkId?: string;
  selectedChunkIds: string[];
  textMode: ChunkTextMode;
  editChunk: (chunkId: string) => void;
  handleCheckboxClick: (chunkId: string, checked: boolean) => void;
  switchChunk: (available?: number, chunkIds?: string[]) => void;
  clickChunkCard: (chunkId: string) => void;
  imageCacheKey?: string | number;
}

// Owned by a separate component so each result set can remount it (key from
// the parent): the virtualizer and its scroll element are born together, so
// a fresh search starts from estimates instead of the previous set's stale
// per-index measured heights that made cards overlap.
function ChunkVirtualList({
  items,
  selectedChunkId,
  selectedChunkIds,
  textMode,
  editChunk,
  handleCheckboxClick,
  switchChunk,
  clickChunkCard,
  imageCacheKey,
}: ChunkVirtualListProps) {
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollContainerRef.current,
    estimateSize: () => 180,
    overscan: 5,
    getItemKey: (index) => items[index]?.chunk_id ?? index,
  });

  return (
    <div ref={scrollContainerRef} className="flex-1 overflow-y-auto min-h-0">
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const item = items[virtualItem.index];
          return (
            <div
              key={virtualItem.key}
              data-index={virtualItem.index}
              ref={virtualizer.measureElement}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${virtualItem.start}px)`,
              }}
              className="pb-4"
            >
              <ChunkCard
                item={item}
                editChunk={editChunk}
                checked={selectedChunkIds.some((x) => x === item.chunk_id)}
                handleCheckboxClick={handleCheckboxClick}
                switchChunk={switchChunk}
                clickChunkCard={clickChunkCard}
                selected={item.chunk_id === selectedChunkId}
                textMode={textMode}
                t={imageCacheKey}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}

export default Chunk;
