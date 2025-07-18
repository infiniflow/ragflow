import CopyToClipboard from '@/components/copy-to-clipboard';
import { Sheet, SheetContent, SheetHeader } from '@/components/ui/sheet';
import { useDebugSingle } from '@/hooks/flow-hooks';
import { useFetchInputForm } from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import { isEmpty } from 'lodash';
import { X } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import DebugContent from '../../debug-content';
import { buildBeginInputListFromObject } from '../../form/begin-form/utils';

interface IProps {
  componentId?: string;
}

const SingleDebugSheet = ({
  componentId,
  visible,
  hideModal,
}: IModalProps<any> & IProps) => {
  const { t } = useTranslation();
  const inputForm = useFetchInputForm(componentId);
  const { debugSingle, data, loading } = useDebugSingle();

  const list = useMemo(() => {
    return buildBeginInputListFromObject(inputForm);
  }, [inputForm]);

  const onOk = useCallback(
    (nextValues: any[]) => {
      if (componentId) {
        debugSingle({ component_id: componentId, params: nextValues });
      }
    },
    [componentId, debugSingle],
  );

  const content = JSON.stringify(data, null, 2);

  return (
    <Sheet open={visible} modal={false}>
      <SheetContent className="top-20 p-0" closeIcon={false}>
        <SheetHeader className="py-2 px-5">
          <div className="flex justify-between ">
            {t('flow.testRun')}
            <X onClick={hideModal} className="cursor-pointer" />
          </div>
        </SheetHeader>
        <section className="overflow-y-auto pt-4 px-5">
          <DebugContent
            parameters={list}
            ok={onOk}
            isNext={false}
            loading={loading}
            submitButtonDisabled={list.length === 0}
          ></DebugContent>
          {!isEmpty(data) ? (
            <div className="mt-4 rounded-md bg-slate-200 border border-neutral-200">
              <div className="flex justify-between p-2">
                <span>JSON</span>
                <CopyToClipboard text={content}></CopyToClipboard>
              </div>
              <JsonView
                src={data}
                displaySize
                collapseStringsAfterLength={100000000000}
                className="w-full h-[800px] break-words overflow-auto p-2 bg-slate-100"
              />
            </div>
          ) : null}
        </section>
      </SheetContent>
    </Sheet>
  );
};

export default SingleDebugSheet;
