import CopyToClipboard from '@/components/copy-to-clipboard';
import { Sheet, SheetContent, SheetHeader } from '@/components/ui/sheet';
import { Switch } from '@/components/ui/switch';
import { useFetchInputForm } from '@/hooks/use-agent-request';
import { useDebugSingle } from '@/hooks/use-single-debug-request';
import { IModalProps } from '@/interfaces/common';
import { ICodeForm } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { X } from 'lucide-react';
import { useCallback, useEffect, useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import DebugContent from '../../debug-content';
import { Operator } from '../../constant';
import { useCanvasReadonly } from '../../context';
import { transferInputsArrayToObject } from '../../form/begin-form/use-watch-change';
import {
  deserializeCodeOutputContract,
  getBusinessOutputs,
} from '../../form/code-form/utils';
import useGraphStore from '../../store';
import { ModelInputs } from './model-inputs';
import {
  buildDebugInputList,
  groupCodeExecDebugOutput,
  shouldUseCodeExecDebugLayout,
} from './utils';

function DebugRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 rounded-md border bg-background p-3">
      <span className="text-sm text-text-secondary">{label}</span>
      <span className="text-right text-sm text-text-primary">
        {value || '-'}
      </span>
    </div>
  );
}

function DebugJsonCard({
  title,
  value,
  error,
}: {
  title: string;
  value: unknown;
  error?: boolean;
}) {
  return (
    <div
      className={cn('rounded-md border', {
        'border-state-error': error,
      })}
    >
      <div className="flex justify-between p-2">
        <span>{title}</span>
        <CopyToClipboard text={JSON.stringify(value ?? null, null, 2)} />
      </div>
      <JsonView
        src={value ?? null}
        displaySize
        collapseStringsAfterLength={100000000000}
        className="h-[260px] w-full overflow-auto break-words p-2"
        dark
      />
    </div>
  );
}

function DebugTextCard({ title, value }: { title: string; value: string }) {
  return (
    <div className="rounded-md border">
      <div className="flex justify-between p-2">
        <span>{title}</span>
        <CopyToClipboard text={value} />
      </div>
      <pre className="max-h-[260px] overflow-auto whitespace-pre-wrap break-words p-3 text-sm text-text-primary">
        {value || '-'}
      </pre>
    </div>
  );
}

interface IProps {
  componentId?: string;
}

/** Runs one component and offers model-input capture for editable Agent nodes. */
const SingleDebugSheet = ({
  componentId,
  visible,
  hideModal,
}: IModalProps<any> & IProps) => {
  const { t } = useTranslation();
  const readOnly = useCanvasReadonly();
  const captureId = useId();
  const [captureModelInputs, setCaptureModelInputs] = useState(false);
  const getNode = useGraphStore((state) => state.getNode);
  const inputForm = useFetchInputForm(componentId);
  const { debugSingle, data, result, loading, reset } = useDebugSingle(
    componentId,
    visible,
  );
  const node = getNode(componentId);
  const canCapture = node?.data.label === Operator.Agent && !readOnly;
  const shouldUseCodeExecLayout = shouldUseCodeExecDebugLayout(
    node?.data.label,
  );

  const list = useMemo(() => {
    return buildDebugInputList(inputForm, node?.data.form);
  }, [inputForm, node?.data.form]);
  const initialValues = useMemo(
    () => Object.fromEntries(list.map((input, index) => [index, input.value])),
    [list],
  );

  useEffect(() => {
    setCaptureModelInputs(false);
  }, [componentId, visible]);

  /** Clears the active run before closing so late responses stay hidden. */
  const handleClose = useCallback(() => {
    reset();
    hideModal?.();
  }, [hideModal, reset]);

  /** Routes sheet dismissal through the same result cleanup as the close icon. */
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) handleClose();
    },
    [handleClose],
  );

  /** Submits form values with capture enabled only when the node permits it. */
  const onOk = useCallback(
    (nextValues: any[]) => {
      if (componentId) {
        debugSingle({
          component_id: componentId,
          params: transferInputsArrayToObject(nextValues),
          ...(canCapture && captureModelInputs
            ? { include_model_inputs: true }
            : {}),
        });
      }
    },
    [canCapture, captureModelInputs, componentId, debugSingle],
  );

  const formData = shouldUseCodeExecLayout
    ? (node?.data.form as ICodeForm | undefined)
    : undefined;
  const debugData = useMemo(
    () =>
      data && typeof data === 'object'
        ? (data as Record<string, unknown>)
        : undefined,
    [data],
  );
  const { contract, legacyOutputs } = useMemo(
    () => deserializeCodeOutputContract(formData),
    [formData],
  );
  const grouped = useMemo(
    () => groupCodeExecDebugOutput(debugData, contract),
    [contract, debugData],
  );
  const businessOutputPreview = useMemo(() => {
    if (contract?.name && debugData && contract.name in debugData) {
      return { [contract.name]: debugData[contract.name] };
    }

    if (legacyOutputs.length > 0 && debugData) {
      return Object.fromEntries(
        Object.keys(getBusinessOutputs(formData?.outputs)).map((key) => [
          key,
          debugData[key],
        ]),
      );
    }

    return {};
  }, [contract, debugData, formData?.outputs, legacyOutputs.length]);
  const content = JSON.stringify(data, null, 2);
  const hasError = shouldUseCodeExecLayout
    ? !isEmpty(grouped.systemOutputs._ERROR)
    : !isEmpty((data as Record<string, unknown> | undefined)?._ERROR);

  return (
    <Sheet open={visible} onOpenChange={handleOpenChange} modal={false}>
      <SheetContent
        className="top-20 bottom-0 h-auto flex flex-col gap-0 overflow-hidden p-0"
        closeIcon={false}
      >
        <SheetHeader className="shrink-0 px-5 py-2">
          <div className="flex justify-between ">
            {t('flow.testRun')}
            <X onClick={handleClose} className="cursor-pointer" />
          </div>
        </SheetHeader>
        <section className="min-h-0 flex-1 overflow-y-auto px-5 pt-4 pb-5">
          {canCapture && (
            <div className="mb-4 space-y-2">
              <div className="flex items-center justify-between gap-3">
                <span id={captureId}>{t('flow.captureModelInputs')}</span>
                <Switch
                  aria-labelledby={captureId}
                  checked={captureModelInputs}
                  onCheckedChange={setCaptureModelInputs}
                  disabled={loading}
                />
              </div>
              <p className="text-sm text-text-secondary">
                {t('flow.captureModelInputsHint')}
              </p>
            </div>
          )}
          <DebugContent
            key={componentId}
            parameters={list}
            initialValues={initialValues}
            ok={onOk}
            isNext={false}
            loading={loading}
            className="min-h-0 flex-1 overflow-auto pb-5"
            maxHeight="max-h-[83vh]"
          ></DebugContent>
          {result && result.code !== 0 && (
            <div role="alert" className="mt-4 break-words text-state-error">
              {t('flow.testRunError', {
                code: result.code,
                message: result.message,
              })}
            </div>
          )}
          {result?.captureRequested && (
            <div className="mt-4">
              <ModelInputs snapshot={result.modelInputs} />
            </div>
          )}
          {!isEmpty(data) ? (
            <div className="mt-4 space-y-4">
              {shouldUseCodeExecLayout ? (
                <>
                  <div
                    className={cn('space-y-3 rounded-md border p-3', {
                      'border-state-error': hasError,
                    })}
                  >
                    <DebugRow
                      label="Business Output"
                      value={contract?.name || legacyOutputs.join(', ')}
                    />
                    <DebugRow
                      label="Expected Type"
                      value={grouped.expectedType}
                    />
                    <DebugRow label="Actual Type" value={grouped.actualType} />
                  </div>
                  {!isEmpty(businessOutputPreview) && (
                    <DebugJsonCard
                      title="Business Output Value"
                      value={businessOutputPreview}
                      error={hasError}
                    />
                  )}
                  <DebugJsonCard
                    title="Raw Result"
                    value={grouped.rawResult}
                    error={hasError}
                  />
                  <DebugTextCard title="Content" value={grouped.content} />
                  <DebugJsonCard
                    title="System Outputs"
                    value={grouped.systemOutputs}
                    error={hasError}
                  />
                </>
              ) : null}
              <div
                className={cn('rounded-md border', {
                  'border-state-error': hasError,
                })}
              >
                <div className="flex justify-between p-2">
                  <span>
                    {shouldUseCodeExecLayout ? 'Raw Component Output' : 'JSON'}
                  </span>
                  <CopyToClipboard text={content}></CopyToClipboard>
                </div>
                <JsonView
                  src={data}
                  displaySize
                  collapseStringsAfterLength={100000000000}
                  className={cn('w-full overflow-auto break-words p-2', {
                    'h-[520px]': shouldUseCodeExecLayout,
                    'h-[800px]': !shouldUseCodeExecLayout,
                  })}
                  dark
                />
              </div>
            </div>
          ) : null}
        </section>
      </SheetContent>
    </Sheet>
  );
};

export default SingleDebugSheet;
