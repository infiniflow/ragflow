import {
  BulkOperateBar,
  BulkOperateItemType,
} from '@/components/bulk-operate-bar';
import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
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
import { Upload } from 'lucide-react';
import { useEffect, useMemo } from 'react';
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

  const refreshCount = useMemo(() => {
    return documents.findIndex((doc) => doc.run === '1') + documents.length;
  }, [documents]);

  const { data: dataSetData } = useFetchKnowledgeBaseConfiguration({
    refreshCount,
  });
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
            count: documents.length,
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
    <>
      <div className="absolute top-4 right-5">
        <Generate disabled={!(dataSetData.chunk_num > 0)} />
      </div>
      <section className="p-5 min-w-[880px]">
        <ListFilterBar
          title="Dataset"
          onSearchChange={handleInputChange}
          searchString={searchString}
          value={filterValue}
          filterGroup={filterGroup}
          onChange={handleFilterSubmit}
          onOpenChange={onOpenChange}
          filters={filters}
          leftPanel={
            <div className="items-start">
              <div className="pb-1">{t('knowledgeDetails.subbarFiles')}</div>
              <div className="text-text-secondary text-sm">
                {t('knowledgeDetails.datasetDescription')}
              </div>
            </div>
          }
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
              <Button size={'sm'}>
                <Upload />
                {t('knowledgeDetails.addFile')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent className="w-56">
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
            list={updatedList as BulkOperateItemType[]}
            count={selectedCount}
          ></BulkOperateBar>
        )}
        <DatasetTable
          documents={documents}
          pagination={pagination}
          setPagination={setPagination}
          rowSelection={rowSelection}
          setRowSelection={setRowSelection}
          showManageMetadataModal={showManageMetadataModal}
          loading={loading}
        ></DatasetTable>
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
            title={'File Name'}
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
      </section>
    </>
  );
}
