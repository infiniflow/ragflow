import {
  BulkOperateBar,
  BulkOperateItemType,
} from '@/components/bulk-operate-bar';
import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { TruncatedText } from '@/components/truncated-text';
import { PrivilegeManagementDialog } from '@/ee/components/privilege-management/privilege-management-dialog';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useDatasetEditButtonDisabled } from '@/ee/hooks/logic-hooks/use-permission';
import { useGoToPreviousPageOnEmpty } from '@/hooks/logic-hooks';
import { useClearSelectionOnPageChange } from '@/hooks/logic-hooks/use-clear-selection-on-page-change';
import {
  useRowSelection,
  useSelectedIds,
} from '@/hooks/logic-hooks/use-row-selection';
import { useFetchDocumentList } from '@/hooks/use-document-request';
import { LucidePlus } from 'lucide-react';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { MetadataType } from '../components/metedata/constant';
import { useManageMetadata } from '../components/metedata/hooks/use-manage-modal';
import { ManageMetadataModal } from '../components/metedata/manage-modal';
import { useKnowledgeBaseContext } from '../contexts/knowledge-base-context';
import { CancelParsingDropdown } from './cancel-parsing-dropdown';
import { DatasetTable } from './dataset-table';
import { useBulkOperateDataset } from './use-bulk-operate-dataset';
import { useCreateEmptyDocument } from './use-create-empty-document';
import { useSelectDatasetFilters } from './use-select-filters';
import {
  ShowPrivilegeModalContext,
  useShowPrivilegeDialog,
} from '@/ee/pages/dataset/dataset/use-show-privilege-dialog';
import { useHandleUploadDocument } from './use-upload-document';

export default function Dataset() {
  const datasetEditButtonDisabled = useDatasetEditButtonDisabled();

  const { t } = useTranslation();
  const { knowledgeBase } = useKnowledgeBaseContext();
  const {
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
    onDocumentUploadOk,
    documentUploadLoading,
  } = useHandleUploadDocument(knowledgeBase?.pipeline_id);
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

  useGoToPreviousPageOnEmpty(documents?.length, loading);

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

  const {
    rowSelection,
    rowSelectionIsEmpty,
    setRowSelection,
    selectedCount,
    clearRowSelection,
  } = useRowSelection();

  useClearSelectionOnPageChange(pagination, clearRowSelection);

  const {
    handShowPrivilegeModal,
    hidePrivilegeModal,
    privilegeModal,
    recordWithSourceType,
  } = useShowPrivilegeDialog();

  const { list } = useBulkOperateDataset({
    documents,
    rowSelection,
    setRowSelection,
    handShowPrivilegeModal,
  });

  const { selectedIds: selectedRowKeys } = useSelectedIds(
    rowSelection,
    documents,
  );

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
        </div>
      ),
      documentIds: selectedRowKeys,
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

  const datasetDescription = t('knowledgeDetails.datasetDescription');

  return (
    <Card as="article" className="mb-5 mr-5 bg-transparent shadow-none">
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
            <div className="min-w-0">
              <h1 className="leading-normal font-medium truncate">
                {t('knowledgeDetails.subbarFiles')}
              </h1>
              <TruncatedText
                as="p"
                className="text-text-secondary text-sm font-normal truncate"
                tooltip={datasetDescription}
              >
                {datasetDescription}
              </TruncatedText>
            </div>
          }
        >
          <div className="flex items-center gap-4">
            <CancelParsingDropdown />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="default" disabled={datasetEditButtonDisabled}>
                  <LucidePlus />
                  {t('knowledgeDetails.addFile')}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent className="w-auto min-w-40" align="end">
                <DropdownMenuItem onClick={showDocumentUploadModal}>
                  {t('fileManager.uploadFile')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={showCreateModal}>
                  {t('knowledgeDetails.emptyFiles')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
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
        <ShowPrivilegeModalContext.Provider value={{ handShowPrivilegeModal }}>
          <DatasetTable
            documents={documents}
            pagination={pagination}
            setPagination={setPagination}
            rowSelection={rowSelection}
            setRowSelection={setRowSelection}
            showManageMetadataModal={showManageMetadataModal}
            loading={loading}
            bulkOperateBarVisible={!rowSelectionIsEmpty}
          ></DatasetTable>
        </ShowPrivilegeModalContext.Provider>

        {documentUploadVisible && (
          <FileUploadDialog
            hideModal={hideDocumentUploadModal}
            onOk={onDocumentUploadOk}
            loading={documentUploadLoading}
            showParseOnCreation
            isTableParser={knowledgeBase?.chunk_method === 'table'}
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
      </CardContent>
      {/* </section> */}
      {privilegeModal && (
        <PrivilegeManagementDialog
          hideModal={hidePrivilegeModal}
          initialValues={recordWithSourceType}
        ></PrivilegeManagementDialog>
      )}
      {/* </> */}
    </Card>
  );
}
