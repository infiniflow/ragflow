import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useSetModalState } from '@/hooks/common-hooks';
import { Routes } from '@/routes';
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { Plus, Settings, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHandleMenuClick } from '../../sidebar/hooks';
import {
  MetadataDeleteMap,
  MetadataType,
  useManageMetaDataModal,
} from './hooks/use-manage-modal';
import { IManageModalProps, IMetaDataTableData } from './interface';
import { ManageValuesModal } from './manage-values-modal';

export const ManageMetadataModal = (props: IManageModalProps) => {
  const {
    title,
    visible,
    hideModal,
    isDeleteSingleValue,
    tableData: originalTableData,
    isCanAdd,
    type: metadataType,
    otherData,
    isEditField,
    isAddValue,
    isShowDescription = false,
    isShowValueSwitch = false,
    isVerticalShowValue = true,
    success,
  } = props;
  const { t } = useTranslation();
  const [valueData, setValueData] = useState<IMetaDataTableData>({
    field: '',
    description: '',
    values: [],
  });

  const [currentValueIndex, setCurrentValueIndex] = useState<number>(0);
  const [deleteDialogContent, setDeleteDialogContent] = useState({
    visible: false,
    title: '',
    name: '',
    warnText: '',
    onOk: () => {},
    onCancel: () => {},
  });

  const {
    tableData,
    setTableData,
    handleDeleteSingleValue,
    handleDeleteSingleRow,
    handleSave,
    addUpdateValue,
    addDeleteValue,
  } = useManageMetaDataModal(originalTableData, metadataType, otherData);
  const { handleMenuClick } = useHandleMenuClick();
  const {
    visible: manageValuesVisible,
    showModal: showManageValuesModal,
    hideModal: hideManageValuesModal,
  } = useSetModalState();
  const hideDeleteModal = () => {
    setDeleteDialogContent({
      visible: false,
      title: '',
      name: '',
      warnText: '',
      onOk: () => {},
      onCancel: () => {},
    });
  };
  const handAddValueRow = () => {
    setValueData({
      field: '',
      description: '',
      values: [],
    });
    setCurrentValueIndex(tableData.length || 0);
    showManageValuesModal();
  };
  const handleEditValueRow = useCallback(
    (data: IMetaDataTableData, index: number) => {
      setCurrentValueIndex(index);
      setValueData(data);
      showManageValuesModal();
    },
    [showManageValuesModal],
  );

  const columns: ColumnDef<IMetaDataTableData>[] = useMemo(() => {
    const cols: ColumnDef<IMetaDataTableData>[] = [
      {
        accessorKey: 'field',
        header: () => <span>{t('knowledgeDetails.metadata.field')}</span>,
        cell: ({ row }) => (
          <div className="text-sm text-accent-primary">
            {row.getValue('field')}
          </div>
        ),
      },
      {
        accessorKey: 'description',
        header: () => <span>{t('knowledgeDetails.metadata.description')}</span>,
        cell: ({ row }) => (
          <div className="text-sm truncate max-w-32">
            {row.getValue('description')}
          </div>
        ),
      },
      {
        accessorKey: 'values',
        header: () => <span>{t('knowledgeDetails.metadata.values')}</span>,
        cell: ({ row }) => {
          const values = row.getValue('values') as Array<string>;
          return (
            <div className="flex items-center gap-1">
              {Array.isArray(values) &&
                values.length > 0 &&
                values
                  .filter((value: string, index: number) => index < 2)
                  ?.map((value: string) => {
                    return (
                      <Button
                        key={value}
                        variant={'ghost'}
                        className="border border-border-button"
                        aria-label="Edit"
                      >
                        <div className="flex gap-1 items-center">
                          <div className="text-sm truncate max-w-24">
                            {value}
                          </div>
                          {isDeleteSingleValue && (
                            <Button
                              variant={'delete'}
                              className="p-0 bg-transparent"
                              onClick={() => {
                                setDeleteDialogContent({
                                  visible: true,
                                  title:
                                    t('common.delete') +
                                    ' ' +
                                    t('knowledgeDetails.metadata.value'),
                                  name: value,
                                  warnText:
                                    MetadataDeleteMap(t)[
                                      metadataType as MetadataType
                                    ].warnValueText,
                                  onOk: () => {
                                    hideDeleteModal();
                                    handleDeleteSingleValue(
                                      row.getValue('field'),
                                      value,
                                    );
                                  },
                                  onCancel: () => {
                                    hideDeleteModal();
                                  },
                                });
                              }}
                            >
                              <Trash2 />
                            </Button>
                          )}
                        </div>
                      </Button>
                    );
                  })}
              {Array.isArray(values) && values.length > 2 && (
                <div className="text-text-secondary self-end">...</div>
              )}
            </div>
          );
        },
      },
      {
        accessorKey: 'action',
        header: () => <span>{t('knowledgeDetails.metadata.action')}</span>,
        meta: {
          cellClassName: 'w-12',
        },
        cell: ({ row }) => (
          <div className=" flex opacity-0 group-hover:opacity-100 gap-2">
            <Button
              variant={'ghost'}
              className="bg-transparent px-1 py-0"
              onClick={() => {
                handleEditValueRow(row.original, row.index);
              }}
            >
              <Settings />
            </Button>
            <Button
              variant={'delete'}
              className="p-0 bg-transparent"
              onClick={() => {
                setDeleteDialogContent({
                  visible: true,
                  title:
                    // t('common.delete') +
                    // ' ' +
                    // t('knowledgeDetails.metadata.metadata')
                    MetadataDeleteMap(t)[metadataType as MetadataType].title,
                  name: row.getValue('field'),
                  warnText:
                    MetadataDeleteMap(t)[metadataType as MetadataType]
                      .warnFieldText,
                  onOk: () => {
                    hideDeleteModal();
                    handleDeleteSingleRow(row.getValue('field'));
                  },
                  onCancel: () => {
                    hideDeleteModal();
                  },
                });
              }}
            >
              <Trash2 />
            </Button>
          </div>
        ),
      },
    ];
    if (!isShowDescription) {
      cols.splice(1, 1);
    }
    return cols;
  }, [
    handleDeleteSingleRow,
    t,
    handleDeleteSingleValue,
    isShowDescription,
    isDeleteSingleValue,
    handleEditValueRow,
    metadataType,
  ]);

  const table = useReactTable({
    data: tableData as IMetaDataTableData[],
    columns,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    manualPagination: true,
  });
  const [shouldSave, setShouldSave] = useState(false);
  const handleSaveValues = (data: IMetaDataTableData) => {
    setTableData((prev) => {
      let newData;
      if (currentValueIndex >= prev.length) {
        // Add operation
        newData = [...prev, data];
      } else {
        // Edit operation
        newData = prev.map((item, index) => {
          if (index === currentValueIndex) {
            return data;
          }
          return item;
        });
      }

      // Deduplicate by field and merge values
      const fieldMap = new Map<string, IMetaDataTableData>();
      newData.forEach((item) => {
        if (fieldMap.has(item.field)) {
          // Merge values if field exists
          const existingItem = fieldMap.get(item.field)!;
          const mergedValues = [
            ...new Set([...existingItem.values, ...item.values]),
          ];
          fieldMap.set(item.field, { ...existingItem, values: mergedValues });
        } else {
          fieldMap.set(item.field, item);
        }
      });

      return Array.from(fieldMap.values());
    });
    setShouldSave(true);
  };

  useEffect(() => {
    if (shouldSave) {
      const timer = setTimeout(() => {
        handleSave({ callback: () => {} });
        setShouldSave(false);
      }, 0);

      return () => clearTimeout(timer);
    }
  }, [tableData, shouldSave, handleSave]);

  const existsKeys = useMemo(() => {
    return tableData.map((item) => item.field);
  }, [tableData]);

  return (
    <>
      <Modal
        title={title}
        open={visible}
        onCancel={hideModal}
        maskClosable={false}
        okText={t('common.save')}
        onOk={async () => {
          const res = await handleSave({ callback: hideModal });
          console.log('data', res);
          success?.(res);
        }}
      >
        <>
          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <div>{t('knowledgeDetails.metadata.metadata')}</div>
              {metadataType === MetadataType.Manage && (
                <Button
                  variant={'ghost'}
                  className="border border-border-button"
                  type="button"
                  onClick={handleMenuClick(Routes.DataSetSetting, {
                    openMetadata: true,
                  })}
                >
                  {t('knowledgeDetails.metadata.toMetadataSetting')}
                </Button>
              )}
              {isCanAdd && (
                <Button
                  variant={'ghost'}
                  className="border border-border-button"
                  type="button"
                  onClick={handAddValueRow}
                >
                  <Plus />
                </Button>
              )}
            </div>
            <Table rootClassName="max-h-[800px]">
              <TableHeader>
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <TableHead key={header.id}>
                        {header.isPlaceholder
                          ? null
                          : flexRender(
                              header.column.columnDef.header,
                              header.getContext(),
                            )}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody className="relative">
                {table.getRowModel().rows?.length ? (
                  table.getRowModel().rows.map((row) => (
                    <TableRow
                      key={row.id}
                      data-state={row.getIsSelected() && 'selected'}
                      className="group"
                    >
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id}>
                          {flexRender(
                            cell.column.columnDef.cell,
                            cell.getContext(),
                          )}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell
                      colSpan={columns.length}
                      className="h-24 text-center"
                    >
                      <Empty type={EmptyType.Data} />
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
          {metadataType === MetadataType.Manage && (
            <div className=" absolute bottom-6 left-5 text-text-secondary text-sm">
              {t('knowledgeDetails.metadata.toMetadataSettingTip')}
            </div>
          )}
        </>
      </Modal>
      {manageValuesVisible && (
        <ManageValuesModal
          title={
            <div>
              {metadataType === MetadataType.Setting ||
              metadataType === MetadataType.SingleFileSetting
                ? t('knowledgeDetails.metadata.fieldSetting')
                : t('knowledgeDetails.metadata.editMetadata')}
            </div>
          }
          type={metadataType}
          existsKeys={existsKeys}
          visible={manageValuesVisible}
          hideModal={hideManageValuesModal}
          data={valueData}
          onSave={handleSaveValues}
          addUpdateValue={addUpdateValue}
          addDeleteValue={addDeleteValue}
          isEditField={isEditField || isCanAdd}
          isAddValue={isAddValue || isCanAdd}
          isShowDescription={isShowDescription}
          isShowValueSwitch={isShowValueSwitch}
          isVerticalShowValue={isVerticalShowValue}
          //   handleDeleteSingleValue={handleDeleteSingleValue}
          //   handleDeleteSingleRow={handleDeleteSingleRow}
        />
      )}

      {deleteDialogContent.visible && (
        <ConfirmDeleteDialog
          open={deleteDialogContent.visible}
          onCancel={deleteDialogContent.onCancel}
          onOk={deleteDialogContent.onOk}
          title={deleteDialogContent.title}
          content={{
            node: (
              <ConfirmDeleteDialogNode
                name={deleteDialogContent.name}
                warnText={deleteDialogContent.warnText}
              />
            ),
          }}
        />
      )}
    </>
  );
};
