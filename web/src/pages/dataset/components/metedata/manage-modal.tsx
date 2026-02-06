import { BulkOperateBar } from '@/components/bulk-operate-bar';
import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useSetModalState } from '@/hooks/common-hooks';
import { useRowSelection } from '@/hooks/logic-hooks/use-row-selection';
import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { Plus, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHandleMenuClick } from '../../sidebar/hooks';
import {
  getMetadataValueTypeLabel,
  MetadataType,
  metadataValueTypeEnum,
} from './constant';
import {
  useManageMetaDataModal,
  useOperateData,
} from './hooks/use-manage-modal';
import {
  IBuiltInMetadataItem,
  IManageModalProps,
  IMetaDataTableData,
} from './interface';
import { useMetadataColumns } from './manage-modal-column';
import { ManageValuesModal } from './manage-values-modal';

type MetadataSettingsTab = 'generation' | 'built-in';

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
    builtInMetadata,
    success,
    documentIds,
    secondTitle,
  } = props;
  const { t } = useTranslation();
  const [valueData, setValueData] = useState<IMetaDataTableData>({
    field: '',
    description: '',
    values: [],
    valueType: metadataValueTypeEnum.string,
  });

  const [activeTab, setActiveTab] = useState<MetadataSettingsTab>('generation');
  const [currentValueIndex, setCurrentValueIndex] = useState<number>(0);
  const [builtInSelection, setBuiltInSelection] = useState<
    IBuiltInMetadataItem[]
  >([]);

  const {
    tableData,
    setTableData,
    handleDeleteSingleValue,
    handleDeleteSingleRow,
    handleSave,
    addUpdateValue,
    addDeleteValue,
    handleDeleteBatchRow,
  } = useManageMetaDataModal(
    originalTableData,
    metadataType,
    otherData,
    documentIds,
  );
  const { handleMenuClick } = useHandleMenuClick();
  const [shouldSave, setShouldSave] = useState(false);
  const [isAddValueMode, setIsAddValueMode] = useState(false);
  const {
    visible: manageValuesVisible,
    showModal: showManageValuesModal,
    hideModal: hideManageValuesModal,
  } = useSetModalState();

  const hideManageValuesModalFunc = () => {
    setIsAddValueMode(false);
    hideManageValuesModal();
  };

  const isSettingsMode =
    metadataType === MetadataType.Setting ||
    metadataType === MetadataType.SingleFileSetting ||
    metadataType === MetadataType.UpdateSingle;

  const showTypeColumn = isSettingsMode;
  const builtInRows = useMemo(
    () => [
      {
        field: 'update_time',
        valueType: 'time',
        description: t('knowledgeConfiguration.builtIn'),
      },
      {
        field: 'file_name',
        valueType: 'string',
        description: t('knowledgeConfiguration.builtIn'),
      },
    ],
    [t],
  );
  const builtInTypeByKey = useMemo(
    () =>
      new Map(
        builtInRows.map((row) => [
          row.field,
          row.valueType as IBuiltInMetadataItem['type'],
        ]),
      ),
    [builtInRows],
  );

  useEffect(() => {
    if (!visible) return;
    setBuiltInSelection(
      (builtInMetadata || []).map((item) => {
        if (typeof item === 'string') {
          return {
            key: item,
            type: builtInTypeByKey.get(item) || 'string',
          };
        }
        return {
          key: item.key,
          type: (item.type ||
            builtInTypeByKey.get(item.key) ||
            'string') as IBuiltInMetadataItem['type'],
        };
      }),
    );
    setActiveTab('generation');
  }, [builtInMetadata, builtInTypeByKey, visible]);

  const builtInSelectionKeys = useMemo(
    () => new Set(builtInSelection.map((item) => item.key)),
    [builtInSelection],
  );

  const handAddValueRow = () => {
    setValueData({
      field: '',
      description: '',
      values:
        metadataType === MetadataType.Setting ||
        metadataType === MetadataType.SingleFileSetting
          ? []
          : [''],
      valueType: metadataValueTypeEnum.string,
    });
    setCurrentValueIndex(tableData.length || 0);
    setIsAddValueMode(true);
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
  const { rowSelection, rowSelectionIsEmpty, setRowSelection, selectedCount } =
    useRowSelection();
  const { columns, deleteDialogContent } = useMetadataColumns({
    isDeleteSingleValue: !!isDeleteSingleValue,
    metadataType,
    setTableData,
    handleDeleteSingleValue,
    handleDeleteSingleRow,
    handleEditValueRow,
    isShowDescription,
    showTypeColumn,
    setShouldSave,
  });

  const table = useReactTable({
    data: tableData as IMetaDataTableData[],
    columns,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onRowSelectionChange: setRowSelection,
    manualPagination: true,
    state: {
      rowSelection,
    },
  });

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
          fieldMap.set(item.field, {
            ...existingItem,
            ...item,
            values: mergedValues,
          });
        } else {
          fieldMap.set(item.field, item);
        }
      });

      return Array.from(fieldMap.values());
    });
    setShouldSave(true);
    setIsAddValueMode(false);
  };

  useEffect(() => {
    if (shouldSave) {
      const timer = setTimeout(() => {
        handleSave({ callback: () => {}, builtInMetadata: builtInSelection });
        setShouldSave(false);
      }, 0);
      return () => clearTimeout(timer);
    }
  }, [tableData, shouldSave, handleSave, builtInSelection]);

  const existsKeys = useMemo(() => {
    return tableData.map((item) => item.field);
  }, [tableData]);

  const { handleDelete } = useOperateData({
    rowSelection,
    list: tableData,
    handleDeleteBatchRow,
  });

  const operateList = [
    {
      id: 'delete',
      label: t('common.delete'),
      icon: <Trash2 />,
      onClick: async () => {
        await handleDelete();
        setRowSelection({});
        // if (code === 0) {
        //   setRowSelection({});
        // }
      },
    },
  ];
  return (
    <>
      <Modal
        title={title}
        open={visible}
        onCancel={hideModal}
        maskClosable={false}
        okText={t('common.save')}
        onOk={async () => {
          const res = await handleSave({
            callback: hideModal,
            builtInMetadata: builtInSelection,
          });
          success?.(res);
        }}
      >
        <>
          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <div className="w-1/2">
                {secondTitle || t('knowledgeDetails.metadata.metadata')}
              </div>
              <div>
                {/* {metadataType === MetadataType.Manage && (
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
                )} */}
                {isCanAdd && activeTab !== 'built-in' && (
                  <Button
                    variant={'ghost'}
                    className="border border-border-button"
                    type="button"
                    onClick={handAddValueRow}
                  >
                    <Plus />
                    {t('common.add')}
                  </Button>
                )}
              </div>
            </div>

            {rowSelectionIsEmpty || (
              <BulkOperateBar
                list={operateList}
                count={selectedCount}
              ></BulkOperateBar>
            )}
            {metadataType === MetadataType.Setting ||
            metadataType === MetadataType.SingleFileSetting ? (
              <Tabs
                value={activeTab}
                onValueChange={(v) => setActiveTab(v as MetadataSettingsTab)}
              >
                <TabsList className="w-fit">
                  <TabsTrigger value="generation">
                    {t('knowledgeDetails.metadata.generation')}
                  </TabsTrigger>
                  <TabsTrigger value="built-in">
                    {t('knowledgeDetails.metadata.builtIn')}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="generation">
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
                </TabsContent>
                <TabsContent value="built-in">
                  <Table rootClassName="max-h-[800px]">
                    <TableHeader>
                      <TableRow>
                        <TableHead>
                          {t('knowledgeDetails.metadata.field')}
                        </TableHead>
                        <TableHead>Type</TableHead>
                        <TableHead>
                          {t('knowledgeDetails.metadata.description')}
                        </TableHead>
                        <TableHead className="text-right">
                          {t('knowledgeDetails.metadata.action')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody className="relative">
                      {builtInRows.map((row) => (
                        <TableRow key={row.field}>
                          <TableCell>
                            <div className="text-sm text-accent-primary">
                              {row.field}
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className="text-sm">
                              {getMetadataValueTypeLabel(
                                row.valueType as IMetaDataTableData['valueType'],
                              )}
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className="text-sm truncate max-w-32">
                              {row.description}
                            </div>
                          </TableCell>
                          <TableCell className="text-right">
                            <Switch
                              checked={builtInSelectionKeys.has(row.field)}
                              onCheckedChange={(checked) => {
                                setBuiltInSelection((prev) => {
                                  if (checked) {
                                    const nextType =
                                      row.valueType as IBuiltInMetadataItem['type'];
                                    if (
                                      prev.some(
                                        (item) => item.key === row.field,
                                      )
                                    ) {
                                      return prev.map((item) =>
                                        item.key === row.field
                                          ? { ...item, type: nextType }
                                          : item,
                                      );
                                    }
                                    return [
                                      ...prev,
                                      { key: row.field, type: nextType },
                                    ];
                                  }
                                  return prev.filter(
                                    (item) => item.key !== row.field,
                                  );
                                });
                              }}
                            />
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TabsContent>
              </Tabs>
            ) : (
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
            )}
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
          hideModal={hideManageValuesModalFunc}
          data={valueData}
          onSave={handleSaveValues}
          addUpdateValue={addUpdateValue}
          addDeleteValue={addDeleteValue}
          isEditField={isEditField || isAddValueMode}
          isAddValue={isAddValue || isAddValueMode}
          isShowDescription={isShowDescription}
          isShowValueSwitch={isShowValueSwitch}
          isShowType={true}
          isVerticalShowValue={isVerticalShowValue}
          isAddValueMode={isAddValueMode}
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
