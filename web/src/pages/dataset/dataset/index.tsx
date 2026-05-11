import {
  BulkOperateBar,
  BulkOperateItemType,
} from '@/components/bulk-operate-bar';
import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useRowSelection } from '@/hooks/logic-hooks/use-row-selection';
import { useFetchDocumentList } from '@/hooks/use-document-request';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { LucidePlus } from 'lucide-react';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { MetadataType } from '../components/metedata/constant';
import { useManageMetadata } from '../components/metedata/hooks/use-manage-modal';
import { ManageMetadataModal } from '../components/metedata/manage-modal';
import { useKnowledgeBaseContext } from '../contexts/knowledge-base-context';
import { DatasetTable } from './dataset-table';
import Generate from './generate-button/generate';
import { ReparseDialog } from './reparse-dialog';
import { useBulkOperateDataset } from './use-bulk-operate-dataset';
import { useCreateEmptyDocument } from './use-create-empty-document';
import { useSelectDatasetFilters } from './use-select-filters';
import { useHandleUploadDocument } from './use-upload-document';

export default function Dataset() {
  const { t } = useTranslation();
  const {
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
    onDocumentUploadOk,
    documentUploadLoading,
  } = useHandleUploadDocument();
  const { knowledgeBase } = useKnowledgeBaseContext();
  const {
    searchString,
    documents,
    pagination,
    handleInputChange,
    setPagination,
    filterValue,
    handleFilterSubmit,
    loading,
    checkValue,
  } = useFetchDocumentList();

  const { data: dataSetData } = useFetchKnowledgeBaseConfiguration();

  const { filters, onOpenChange, filterGroup } = useSelectDatasetFilters();

  const {
    createLoading,
    onCreateOk,
    createVisible,
    hideCreateModal,
    showCreateModal,
  } = useCreateEmptyDocument();

  const {
    manageMetadataVisible,
    showManageMetadataModal,
    hideManageMetadataModal,
    tableData,
    config: metadataConfig,
  } = useManageMetadata();

  useEffect(() => {
    checkValue(filters);
  }, [filters]);

  const { rowSelection, rowSelectionIsEmpty, setRowSelection, selectedCount } =
    useRowSelection();

  const {
    chunkNum,
    list,
    visible: reparseDialogVisible,
    hideModal: hideReparseDialogModal,
    handleRunClick: handleOperationIconClick,
  } = useBulkOperateDataset({
    documents,
    rowSelection,
    setRowSelection,
  });

  const handleAddMetadataWithDocuments = () => {
    showManageMetadataModal({
      type: MetadataType.Manage,
      isCanAdd: true,
      isEditField: false,
      isDeleteSingleValue: true,
      isAddValue: true,
      secondTitle: (
        <>
          {t('knowledgeDetails.metadata.selectFiles', {
            count: selectedCount,
          })}
        </>
      ),
      title: (
        <div className="flex flex-col gap-2">
          <div className="text-base font-normal">
            {t('knowledgeDetails.metadata.manageMetadata')}
          </div>
          {/* <div className="text-sm text-text-secondary">
            {t('knowledgeDetails.metadata.manageMetadataForDataset')}
          </div> */}
        </div>
      ),
      documentIds: documents.map((doc) => doc.id),
    });
  };

  const updatedList = list.map((item) => {
    if (item.id === 'batch-metadata') {
      return {
        ...item,
        onClick: handleAddMetadataWithDocuments,
      };
    }
    return item;
  });

  return (
    <Card
      as="article"
      className="mb-5 mr-5 min-w-[880px] bg-transparent shadow-none"
    >
      <CardHeader as="header" className="p-5 space-y-0">
        <ListFilterBar
          onSearchChange={handleInputChange}
          searchString={searchString}
          value={filterValue}
          filterGroup={filterGroup}
          onChange={handleFilterSubmit}
          onOpenChange={onOpenChange}
          filters={filters}
          className="items-end"
          leftPanel={
            <div>
              <h1 className="leading-normal font-medium">
                {t('knowledgeDetails.subbarFiles')}
              </h1>
              <p className="text-text-secondary text-sm font-normal">
                {t('knowledgeDetails.datasetDescription')}
              </p>
            </div>
          }
          preChildren={<Generate disabled={!(dataSetData.chunk_num > 0)} />}
          // preChildren={
          //   <Button
          //     variant={'ghost'}
          //     className="border border-border-button"
          //     onClick={() =>
          //       showManageMetadataModal({
          //         type: MetadataType.Manage,
          //         isCanAdd: false,
          //         isEditField: false,
          //         isDeleteSingleValue: true,
          //         title: (
          //           <div className="flex flex-col gap-2">
          //             <div className="text-base font-normal">
          //               {t('knowledgeDetails.metadata.manageMetadata')}
          //             </div>
          //             <div className="text-sm text-text-secondary">
          //               {t(
          //                 'knowledgeDetails.metadata.manageMetadataForDataset',
          //               )}
          //             </div>
          //           </div>
          //         ),
          //       })
          //     }
          //   >
          //     <div className="flex gap-1 items-center">
          //       <Pen size={14} />
          //       {t('knowledgeDetails.metadata.metadata')}
          //     </div>
          //   </Button>
          // }
        >
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="default">
                <LucidePlus />
                {t('knowledgeDetails.addFile')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent className="w-auto min-w-40" align="end">
              <DropdownMenuItem onClick={showDocumentUploadModal}>
                {t('fileManager.uploadFile')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={showCreateModal}>
                {t('knowledgeDetails.emptyFiles')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </ListFilterBar>

        {rowSelectionIsEmpty || (
          <BulkOperateBar
            className="!mt-2.5 !-mb-2.5"
            list={updatedList as BulkOperateItemType[]}
            count={selectedCount}
          />
        )}
      </CardHeader>

      <CardContent className="px-5 py-0">
        <DatasetTable
          documents={documents}
          pagination={pagination}
          setPagination={setPagination}
          rowSelection={rowSelection}
          setRowSelection={setRowSelection}
          showManageMetadataModal={showManageMetadataModal}
          loading={loading}
        />

        {documentUploadVisible && (
          <FileUploadDialog
            hideModal={hideDocumentUploadModal}
            onOk={onDocumentUploadOk}
            loading={documentUploadLoading}
            showParseOnCreation
          ></FileUploadDialog>
        )}
        {createVisible && (
          <RenameDialog
            hideModal={hideCreateModal}
            onOk={onCreateOk}
            loading={createLoading}
            title={t('knowledgeDetails.fileName')}
          ></RenameDialog>
        )}
        {manageMetadataVisible && (
          <ManageMetadataModal
            title={
              metadataConfig.title || (
                <div className="flex flex-col gap-2">
                  <div className="text-base font-normal">
                    {t('knowledgeDetails.metadata.manageMetadata')}
                  </div>
                  {/* <div className="text-sm text-text-secondary">
                    {t('knowledgeDetails.metadata.manageMetadataForDataset')}
                  </div> */}
                </div>
              )
            }
            visible={manageMetadataVisible}
            hideModal={() => {
              setRowSelection({});
              hideManageMetadataModal();
            }}
            // selectedRowKeys={selectedRowKeys}
            tableData={tableData}
            isCanAdd={metadataConfig.isCanAdd}
            isAddValue={metadataConfig.isAddValue}
            isVerticalShowValue={metadataConfig.isVerticalShowValue}
            isEditField={metadataConfig.isEditField}
            isDeleteSingleValue={metadataConfig.isDeleteSingleValue}
            secondTitle={metadataConfig.secondTitle}
            type={metadataConfig.type}
            documentIds={metadataConfig.documentIds}
            otherData={metadataConfig.record}
          />
        )}
        {reparseDialogVisible && (
          <ReparseDialog
            hidden={
              chunkNum === 0 && !knowledgeBase?.parser_config?.enable_metadata
            }
            // hidden={false}
            enable_metadata={knowledgeBase?.parser_config?.enable_metadata}
            handleOperationIconClick={handleOperationIconClick}
            chunk_num={chunkNum}
            visible={reparseDialogVisible}
            hideModal={hideReparseDialogModal}
          ></ReparseDialog>
        )}
      </CardContent>
    </Card>
  );
}
