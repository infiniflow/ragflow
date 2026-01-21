import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { ColumnDef, Row, Table } from '@tanstack/react-table';
import {
  ListChevronsDownUp,
  ListChevronsUpDown,
  Settings,
  Trash2,
} from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  getMetadataValueTypeLabel,
  MetadataDeleteMap,
  MetadataType,
} from './constant';
import { IMetaDataTableData } from './interface';

interface IUseMetadataColumns {
  isDeleteSingleValue: boolean;
  metadataType: MetadataType;
  handleDeleteSingleValue: (field: string, value: string) => void;
  handleDeleteSingleRow: (field: string) => void;
  handleEditValueRow: (data: IMetaDataTableData, index: number) => void;
  showTypeColumn: boolean;
  isShowDescription: boolean;
  setTableData: React.Dispatch<React.SetStateAction<IMetaDataTableData[]>>;
  setShouldSave: React.Dispatch<React.SetStateAction<boolean>>;
}

export const useMetadataColumns = ({
  isDeleteSingleValue,
  metadataType,
  handleDeleteSingleValue,
  handleDeleteSingleRow,
  handleEditValueRow,
  isShowDescription,
  setTableData,
  setShouldSave,
}: IUseMetadataColumns) => {
  const { t } = useTranslation();
  const [deleteDialogContent, setDeleteDialogContent] = useState({
    visible: false,
    title: '',
    name: '',
    warnText: '',
    onOk: () => {},
    onCancel: () => {},
  });
  const [expanded, setExpanded] = useState(true);
  const [editingValue, setEditingValue] = useState<{
    field: string;
    value: string;
    newValue: string;
  } | null>(null);
  const isSettingsMode =
    metadataType === MetadataType.Setting ||
    metadataType === MetadataType.SingleFileSetting ||
    metadataType === MetadataType.UpdateSingle;

  const showTypeColumn = isSettingsMode;
  const handleEditValue = (field: string, value: string) => {
    setEditingValue({ field, value, newValue: value });
  };

  const saveEditedValue = useCallback(() => {
    if (editingValue) {
      setTableData((prev) => {
        return prev.map((row) => {
          if (row.field === editingValue.field) {
            const updatedValues = row.values.map((v) =>
              v === editingValue.value ? editingValue.newValue : v,
            );
            return { ...row, values: updatedValues };
          }
          return row;
        });
      });
      setEditingValue(null);
      setShouldSave(true);
    }
  }, [editingValue, setTableData]);

  const cancelEditValue = () => {
    setEditingValue(null);
  };
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
  const columns = useMemo(() => {
    const cols: ColumnDef<IMetaDataTableData>[] = [
      ...(MetadataType.Manage === metadataType ||
      MetadataType.UpdateSingle === metadataType
        ? [
            {
              id: 'select',
              header: ({ table }: { table: Table<IMetaDataTableData> }) => (
                <Checkbox
                  checked={
                    table.getIsAllPageRowsSelected() ||
                    (table.getIsSomePageRowsSelected() && 'indeterminate')
                  }
                  onCheckedChange={(value) =>
                    table.toggleAllPageRowsSelected(!!value)
                  }
                  aria-label="Select all"
                />
              ),
              cell: ({ row }: { row: Row<IMetaDataTableData> }) => (
                <Checkbox
                  checked={row.getIsSelected()}
                  onCheckedChange={(value) => row.toggleSelected(!!value)}
                  aria-label="Select row"
                />
              ),
              enableSorting: false,
              enableHiding: false,
            },
          ]
        : []),
      {
        accessorKey: 'field',
        header: () => <span>{t('knowledgeDetails.metadata.field')}</span>,
        cell: ({ row }) => (
          <div className="text-sm text-accent-primary">
            {row.getValue('field')}
          </div>
        ),
      },
      // ...(showTypeColumn
      //   ? ([
      //       {
      //         accessorKey: 'valueType',
      //         header: () => <span>Type</span>,
      //         cell: ({ row }) => (
      //           <div className="text-sm">
      //             {getMetadataValueTypeLabel(
      //               row.original.valueType as IMetaDataTableData['valueType'],
      //             )}
      //           </div>
      //         ),
      //       },
      //     ] as ColumnDef<IMetaDataTableData>[])
      //   : []),
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
        accessorKey: 'valueType',
        header: () => <span>{t('knowledgeDetails.metadata.type')}</span>,
        cell: ({ row }) => (
          <div className="text-sm">
            {getMetadataValueTypeLabel(
              row.original.valueType as IMetaDataTableData['valueType'],
            )}
          </div>
        ),
      },
      {
        accessorKey: 'values',
        header: () => (
          <div className="flex items-center">
            <span>{t('knowledgeDetails.metadata.values')}</span>
            <div
              className="ml-2 p-1 cursor-pointer"
              onClick={() => {
                setExpanded(!expanded);
              }}
            >
              {expanded ? (
                <ListChevronsDownUp size={14} />
              ) : (
                <ListChevronsUpDown size={14} />
              )}
              {expanded}
            </div>
          </div>
        ),
        cell: ({ row }) => {
          const values = row.getValue('values') as Array<string>;
          //   const supportsEnum = isMetadataValueTypeWithEnum(
          //     row.original.valueType,
          //   );

          // if (!supportsEnum || !Array.isArray(values) || values.length === 0) {
          //   return <div></div>;
          // }

          const displayedValues = expanded ? values : values.slice(0, 2);
          const hasMore = Array.isArray(values) && values.length > 2;

          return (
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap gap-1">
                {displayedValues?.map((value: string) => {
                  const isEditing =
                    editingValue &&
                    editingValue.field === row.getValue('field') &&
                    editingValue.value === value;

                  return isEditing ? (
                    <div key={value}>
                      <Input
                        type="text"
                        value={editingValue.newValue}
                        onChange={(e) =>
                          setEditingValue({
                            ...editingValue,
                            newValue: e.target.value,
                          })
                        }
                        onBlur={saveEditedValue}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            saveEditedValue();
                          } else if (e.key === 'Escape') {
                            cancelEditValue();
                          }
                        }}
                        autoFocus
                        // className="text-sm min-w-20 max-w-32 outline-none bg-transparent px-1 py-0.5"
                      />
                    </div>
                  ) : (
                    <Button
                      key={value}
                      variant={'ghost'}
                      className="border border-border-button"
                      onClick={() =>
                        handleEditValue(row.getValue('field'), value)
                      }
                      aria-label="Edit"
                    >
                      <div className="flex gap-1 items-center">
                        <div className="text-sm truncate max-w-24">{value}</div>
                        {isDeleteSingleValue && (
                          <Button
                            variant={'delete'}
                            className="p-0 bg-transparent"
                            onClick={(e) => {
                              e.stopPropagation();
                              //   showDeleteDialogContent(value, row);
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
                {hasMore && !expanded && (
                  <div className="text-text-secondary self-end">...</div>
                )}
              </div>
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
      return cols.filter((col) => {
        if ('accessorKey' in col && col.accessorKey === 'description') {
          return false;
        }
        return true;
      });
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
    expanded,
    editingValue,
    saveEditedValue,
    showTypeColumn,
  ]);

  return {
    columns,
    deleteDialogContent,
  };
};
