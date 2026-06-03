import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { IModalProps } from '@/interfaces/common';
import { extractTableColumns, isTableFile } from '@/utils/table-column-extract';
import { zodResolver } from '@hookform/resolvers/zod';
import { TFunction } from 'i18next';
import { useCallback, useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { FileUploader } from '../file-uploader';
import { RAGFlowFormItem } from '../ragflow-form';
import { Form } from '../ui/form';
import { Label } from '../ui/label';
import { RadioGroup, RadioGroupItem } from '../ui/radio-group';
import { Switch } from '../ui/switch';

const ROLE_OPTIONS = [
  { value: 'both', labelKey: 'knowledgeConfiguration.tableColumnRoleBoth' },
  {
    value: 'indexing',
    labelKey: 'knowledgeConfiguration.tableColumnRoleIndexing',
  },
  {
    value: 'metadata',
    labelKey: 'knowledgeConfiguration.tableColumnRoleMetadata',
  },
] as const;

export type TableColumnRoles = Record<string, 'indexing' | 'metadata' | 'both'>;

function buildUploadFormSchema(t: TFunction) {
  const FormSchema = z.object({
    parseOnCreation: z.boolean().optional(),
    // Update schema to allow files with path property to handle folder uploads
    fileList: z
      .array(
        z.instanceof(File).or(
          z.object({
            file: z.instanceof(File),
            path: z.string(), // Store the relative path for files in folders
          }),
        ),
      )
      .min(1, { message: t('fileManager.pleaseUploadAtLeastOneFile') }),
    tableColumnMode: z.enum(['auto', 'manual']).optional(),
    tableColumnRoles: z
      .record(z.enum(['indexing', 'metadata', 'both']))
      .optional(),
  });

  return FormSchema;
}

export type UploadFormSchemaType = z.infer<
  ReturnType<typeof buildUploadFormSchema>
>;

const UploadFormId = 'UploadFormId';

type UploadFormProps = {
  submit: (values?: UploadFormSchemaType) => void;
  showParseOnCreation?: boolean;
  isTableParser?: boolean;
};
function UploadForm({
  submit,
  showParseOnCreation,
  isTableParser,
}: UploadFormProps) {
  const { t } = useTranslation();
  const FormSchema = buildUploadFormSchema(t);

  type UploadFormSchemaType = z.infer<typeof FormSchema>;
  const form = useForm<UploadFormSchemaType>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      parseOnCreation: false,
      fileList: [],
      tableColumnMode: 'auto',
      tableColumnRoles: {},
    },
  });

  const [extractedColumns, setExtractedColumns] = useState<string[]>([]);
  const [columnMode, setColumnMode] = useState<'auto' | 'manual'>('auto');
  const [columnRoles, setColumnRoles] = useState<TableColumnRoles>({});

  const handleFilesChange = useCallback(
    async (files: any[]) => {
      if (!isTableParser || !files || files.length === 0) {
        setExtractedColumns([]);
        return;
      }

      // Extract columns from the first table file
      const allColumns = new Set<string>();
      for (const f of files) {
        const file = f instanceof File ? f : f.file;
        if (file && isTableFile(file)) {
          const cols = await extractTableColumns(file);
          cols.forEach((c) => allColumns.add(c));
        }
      }
      setExtractedColumns(Array.from(allColumns));
    },
    [isTableParser],
  );

  const handleModeChange = (value: 'auto' | 'manual') => {
    setColumnMode(value);
    form.setValue('tableColumnMode', value);
  };

  const handleRoleChange = (col: string, role: string) => {
    const updated = {
      ...columnRoles,
      [col]: role as 'indexing' | 'metadata' | 'both',
    };
    setColumnRoles(updated);
    form.setValue('tableColumnRoles', updated);
  };

  // Sync column roles to form when columns are extracted
  useEffect(() => {
    if (columnMode === 'manual' && extractedColumns.length > 0) {
      const roles: TableColumnRoles = {};
      extractedColumns.forEach((col) => {
        roles[col] = columnRoles[col] || 'both';
      });
      setColumnRoles(roles);
      form.setValue('tableColumnRoles', roles);
    }
  }, [extractedColumns, columnMode]); // eslint-disable-line react-hooks/exhaustive-deps

  const showColumnConfig = isTableParser && extractedColumns.length > 0;

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(submit)}
        id={UploadFormId}
        className="space-y-4"
      >
        {showParseOnCreation && (
          <RAGFlowFormItem
            name="parseOnCreation"
            label={t('fileManager.parseOnCreation')}
          >
            {(field) => (
              <Switch
                data-testid="parse-on-creation-toggle"
                onCheckedChange={field.onChange}
                checked={field.value}
              />
            )}
          </RAGFlowFormItem>
        )}
        <RAGFlowFormItem name="fileList" label={''}>
          {(field) => (
            <FileUploader
              value={field.value}
              onValueChange={(files) => {
                field.onChange(files);
                handleFilesChange(files);
              }}
              accept={{}}
              data-testid="dataset-upload-dropzone"
            />
          )}
        </RAGFlowFormItem>

        {showColumnConfig && (
          <div className="space-y-3 border rounded-md p-3">
            <div className="space-y-2">
              <Label className="text-sm font-medium">
                {t('knowledgeConfiguration.tableColumnMode')}
              </Label>
              <RadioGroup
                value={columnMode}
                onValueChange={handleModeChange}
                className="flex gap-4"
              >
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="auto" id="upload-mode-auto" />
                  <label
                    htmlFor="upload-mode-auto"
                    className="text-sm font-normal cursor-pointer"
                  >
                    {t('knowledgeConfiguration.tableColumnModeAuto')}
                  </label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="manual" id="upload-mode-manual" />
                  <label
                    htmlFor="upload-mode-manual"
                    className="text-sm font-normal cursor-pointer"
                  >
                    {t('knowledgeConfiguration.tableColumnModeManual')}
                  </label>
                </div>
              </RadioGroup>
            </div>

            {columnMode === 'auto' && (
              <p className="text-sm text-muted-foreground">
                {t('knowledgeConfiguration.tableColumnModeAutoDescription')}
              </p>
            )}

            {columnMode === 'manual' && (
              <div className="space-y-2">
                <p className="text-sm text-muted-foreground">
                  {t('knowledgeConfiguration.tableColumnRolesTip')}
                </p>
                <div className="space-y-2 max-h-[200px] overflow-y-auto">
                  {extractedColumns.map((col) => (
                    <div key={col} className="flex items-center gap-3">
                      <Label className="min-w-[120px] shrink-0 text-sm font-normal truncate">
                        {col}
                      </Label>
                      <Select
                        value={columnRoles[col] || 'both'}
                        onValueChange={(value) => handleRoleChange(col, value)}
                      >
                        <SelectTrigger className="w-[140px]">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {ROLE_OPTIONS.map((opt) => (
                            <SelectItem key={opt.value} value={opt.value}>
                              {t(opt.labelKey)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </form>
    </Form>
  );
}

type FileUploadDialogProps = IModalProps<UploadFormSchemaType> &
  Pick<UploadFormProps, 'showParseOnCreation' | 'isTableParser'>;
export function FileUploadDialog({
  hideModal,
  onOk,
  loading,
  showParseOnCreation = false,
  isTableParser = false,
}: FileUploadDialogProps) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent
        data-testid="dataset-upload-modal"
        className="max-h-[85vh] overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>{t('fileManager.uploadFile')}</DialogTitle>
        </DialogHeader>
        <UploadForm
          submit={onOk!}
          showParseOnCreation={showParseOnCreation}
          isTableParser={isTableParser}
        />
        <DialogFooter>
          <ButtonLoading type="submit" loading={loading} form={UploadFormId}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
