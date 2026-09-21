/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { TableColumnSettingsFields } from '@/components/table-column-settings-form-fields';
import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IModalProps } from '@/interfaces/common';
import { extractTableColumns, isTableFile } from '@/utils/table-column-extract';
import {
  TableColumnRole,
  TableColumnSettings,
} from '@/utils/table-column-settings';
import { zodResolver } from '@hookform/resolvers/zod';
import { TFunction } from 'i18next';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { FileUploader } from '../file-uploader';
import { RAGFlowFormItem } from '../ragflow-form';
import { Form } from '../ui/form';
import { Switch } from '../ui/switch';

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
    tableColumnNamesByFile: z.array(z.array(z.string())).optional(),
    tableColumnRoles: z
      .record(z.enum(['indexing', 'metadata', 'both']))
      .optional(),
  });

  return FormSchema;
}

export type UploadFormSchemaType = z.infer<
  ReturnType<typeof buildUploadFormSchema>
>;

function sameTableColumnRoles(
  a: TableColumnSettings['roles'],
  b: TableColumnSettings['roles'],
): boolean {
  const aKeys = Object.keys(a);
  return (
    aKeys.length === Object.keys(b).length &&
    aKeys.every((key) => a[key] === b[key])
  );
}

const UploadFormId = 'UploadFormId';

type UploadFormProps = {
  submit: (values?: UploadFormSchemaType) => void;
  showParseOnCreation?: boolean;
  isTableParser?: boolean;
  // Dataset-level table column settings, shown as the initial selection so an
  // untouched dialog displays what ingestion will use. They are deliberately
  // NOT submitted: only a mode/role the user changed in this dialog is sent,
  // otherwise the document would pin the dataset's default and later dataset
  // changes could never reach it.
  defaultTableColumnSettings?: TableColumnSettings;
};
function UploadForm({
  submit,
  showParseOnCreation,
  isTableParser,
  defaultTableColumnSettings,
}: UploadFormProps) {
  const { t } = useTranslation();
  const FormSchema = buildUploadFormSchema(t);

  type UploadFormSchemaType = z.infer<typeof FormSchema>;
  const form = useForm<UploadFormSchemaType>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      parseOnCreation: false,
      fileList: [],
      tableColumnNamesByFile: [],
      tableColumnRoles: {},
    },
  });

  const [extractedColumns, setExtractedColumns] = useState<string[]>([]);
  const [columnMode, setColumnMode] = useState<'auto' | 'manual'>(
    defaultTableColumnSettings?.mode ?? 'auto',
  );
  const [columnRoles, setColumnRoles] = useState<TableColumnSettings['roles']>(
    defaultTableColumnSettings?.roles ?? {},
  );
  // Guards the async column-extraction loop: rapid file-list changes must not
  // let a stale extraction overwrite the latest selection.
  const extractionVersion = useRef(0);

  const handleFilesChange = useCallback(
    async (files: any[]) => {
      const version = ++extractionVersion.current;
      if (!isTableParser || !files || files.length === 0) {
        setExtractedColumns([]);
        form.setValue('tableColumnNamesByFile', []);
        return;
      }

      const allColumns = new Set<string>();
      const columnsByFile: string[][] = [];
      for (const f of files) {
        const file = f instanceof File ? f : f.file;
        if (file && isTableFile(file)) {
          const cols = await extractTableColumns(file);
          cols.forEach((c) => allColumns.add(c));
          columnsByFile.push(cols);
        } else {
          columnsByFile.push([]);
        }
      }
      const columns = Array.from(allColumns);
      if (version !== extractionVersion.current) {
        return;
      }
      setExtractedColumns(columns);
      form.setValue('tableColumnNamesByFile', columnsByFile);
    },
    [form, isTableParser],
  );

  const handleModeChange = (value: 'auto' | 'manual') => {
    setColumnMode(value);
    form.setValue('tableColumnMode', value);
  };

  const handleRoleChange = (col: string, role: TableColumnRole) => {
    const updated = { ...columnRoles, [col]: role };
    setColumnRoles(updated);
    form.setValue('tableColumnRoles', updated);
  };

  // A manual selection covers every column the current file list has: seed the
  // ones just discovered and drop the ones a changed list removed. The equality
  // guard is what lets this effect name the roles it reads — without it, writing
  // a fresh object would re-trigger itself forever.
  useEffect(() => {
    if (columnMode !== 'manual' || extractedColumns.length === 0) {
      return;
    }
    const seeded: TableColumnSettings['roles'] = {};
    extractedColumns.forEach((col) => {
      seeded[col] = columnRoles[col] ?? 'both';
    });
    if (sameTableColumnRoles(seeded, columnRoles)) {
      return;
    }
    setColumnRoles(seeded);
    form.setValue('tableColumnRoles', seeded);
  }, [extractedColumns, columnMode, columnRoles, form]);

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
            <TableColumnSettingsFields
              idPrefix="upload"
              mode={columnMode}
              columns={extractedColumns}
              roles={columnRoles}
              onModeChange={handleModeChange}
              onRoleChange={handleRoleChange}
            />
          </div>
        )}
      </form>
    </Form>
  );
}

type FileUploadDialogProps = IModalProps<UploadFormSchemaType> &
  Pick<
    UploadFormProps,
    'showParseOnCreation' | 'isTableParser' | 'defaultTableColumnSettings'
  >;
export function FileUploadDialog({
  hideModal,
  onOk,
  loading,
  showParseOnCreation = false,
  isTableParser = false,
  defaultTableColumnSettings,
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
          defaultTableColumnSettings={defaultTableColumnSettings}
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
