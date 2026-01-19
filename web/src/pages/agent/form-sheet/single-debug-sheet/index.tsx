import CopyToClipboard from '@/components/copy-to-clipboard';
import { Sheet, SheetContent, SheetHeader } from '@/components/ui/sheet';
import { useDebugSingle, useFetchInputForm } from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { X } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import DebugContent from '../../debug-content';
import { transferInputsArrayToObject } from '../../form/begin-form/use-watch-change';
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
        debugSingle({
          component_id: componentId,
          params: transferInputsArrayToObject(nextValues),
        });
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
            <div
              className={cn('mt-4 rounded-md border', {
                [`border-state-error`]: !isEmpty(data._ERROR),
              })}
            >
              <div className="flex justify-between p-2">
                <span>JSON</span>
                <CopyToClipboard text={content}></CopyToClipboard>
              </div>
              <JsonView
                src={data}
                displaySize
                collapseStringsAfterLength={100000000000}
                className="w-full h-[800px] break-words overflow-auto p-2"
                dark
              />
            </div>
          ) : null}
        </section>
      </SheetContent>
    </Sheet>
  );
};

export default SingleDebugSheet;
