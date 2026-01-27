import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { DateInput } from '@/components/ui/input-date';
import { formatDate } from '@/utils/date';
import { ColumnDef, Row, Table } from '@tanstack/react-table';
import { ListChevronsDownUp, Settings, Trash2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  getMetadataValueTypeLabel,
  MetadataDeleteMap,
  MetadataType,
  metadataValueTypeEnum,
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
  // const [expanded, setExpanded] = useState(true);
  const [editingValue, setEditingValue] = useState<{
    field: string;
    value: string;
    newValue: string;
  } | null>(null);
  const [rowExpandedStates, setRowExpandedStates] = useState<
    Record<string, boolean>
  >({});

  const isSettingsMode =
    metadataType === MetadataType.Setting ||
    metadataType === MetadataType.SingleFileSetting ||
    metadataType === MetadataType.UpdateSingle;

  const showTypeColumn = isSettingsMode;
  const handleEditValue = (field: string, value: string) => {
    setEditingValue({ field, value, newValue: value });
  };

  const saveEditedValue = useCallback(
    (newValue?: { field: string; value: string; newValue: string }) => {
      const realValue = newValue || editingValue;
      if (realValue) {
        setTableData((prev) => {
          return prev.map((row) => {
            if (row.field === realValue.field) {
              const updatedValues = row.values.map((v) =>
                v === realValue.value ? realValue.newValue : v,
              );
              return { ...row, values: updatedValues };
            }
            return row;
          });
        });
        setEditingValue(null);
        setShouldSave(true);
      }
    },
    [editingValue, setTableData, setShouldSave],
  );

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
            {/* <div
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
            </div> */}
          </div>
        ),
        cell: ({ row }) => {
          const values = row.getValue('values') as Array<string>;
          const isRowExpanded = rowExpandedStates[row.original.field] ?? false;

          const toggleRowExpanded = () => {
            setRowExpandedStates((prev) => ({
              ...prev,
              [row.original.field]: !isRowExpanded,
            }));
          };

          const displayedValues = isRowExpanded ? values : values.slice(0, 2);
          const hasMore = Array.isArray(values) && values.length > 2;

          return (
            <div className="flex gap-1">
              <div className="flex flex-wrap gap-1">
                {displayedValues?.map((value: string) => {
                  const isEditing =
                    editingValue &&
                    editingValue.field === row.getValue('field') &&
                    editingValue.value === value;

                  return isEditing ? (
                    <div key={value}>
                      {row.original.valueType ===
                        metadataValueTypeEnum.time && (
                        <DateInput
                          value={new Date(editingValue.newValue)}
                          onChange={(value) => {
                            const newValue = {
                              ...editingValue,
                              newValue: formatDate(
                                value,
                                'YYYY-MM-DDTHH:mm:ss',
                              ),
                            };
                            setEditingValue(newValue);
                            saveEditedValue(newValue);
                          }}
                          showTimeSelect={true}
                        />
                      )}
                      {row.original.valueType !==
                        metadataValueTypeEnum.time && (
                        <Input
                          type="text"
                          value={editingValue.newValue}
                          onChange={(e) =>
                            setEditingValue({
                              ...editingValue,
                              newValue: e.target.value,
                            })
                          }
                          onBlur={() => saveEditedValue()}
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
                      )}
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
                        <div className="text-sm truncate max-w-24">
                          {row.original.valueType === metadataValueTypeEnum.time
                            ? formatDate(value, 'DD/MM/YYYY HH:mm:ss')
                            : value}
                        </div>
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
              </div>
              {hasMore && !isRowExpanded && (
                <Button
                  variant={'ghost'}
                  className="border border-border-button h-auto px-2 py-1"
                  onClick={(e) => {
                    e.stopPropagation();
                    toggleRowExpanded();
                  }}
                >
                  <div className="text-text-secondary">
                    +{values.length - 2}
                  </div>
                </Button>
              )}
              {hasMore && isRowExpanded && (
                // <div className="self-end mt-1">
                <Button
                  variant={'ghost'}
                  className="bg-transparent px-2 py-1"
                  onClick={(e) => {
                    e.stopPropagation();
                    toggleRowExpanded();
                  }}
                >
                  <div className="text-text-secondary">
                    <ListChevronsDownUp size={14} />
                  </div>
                </Button>
                // </div>
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
    // expanded,
    editingValue,
    saveEditedValue,
    rowExpandedStates,
  ]);

  return {
    columns,
    deleteDialogContent,
  };
};
