import CopyToClipboard from '@/components/copy-to-clipboard';
import { useDebugSingle, useFetchInputElements } from '@/hooks/flow-hooks';
import { IModalProps } from '@/interfaces/common';
import { CloseOutlined } from '@ant-design/icons';
import { loader } from '@monaco-editor/react';
import { Drawer } from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import DebugContent from '../../debug-content';

loader.config({ paths: { vs: '/vs' } });

interface IProps {
  componentId?: string;
}

const SingleDebugDrawer = ({
  componentId,
  visible,
  hideModal,
}: IModalProps<any> & IProps) => {
  const { t } = useTranslation();
  const { data: list } = useFetchInputElements(componentId);
  const { debugSingle, data } = useDebugSingle();

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
    <Drawer
      title={
        <div className="flex justify-between">
          {t('flow.testRun')}
          <CloseOutlined onClick={hideModal} />
        </div>
      }
      width={'100%'}
      onClose={hideModal}
      open={visible}
      getContainer={false}
      mask={false}
      placement={'bottom'}
      height={'95%'}
      closeIcon={null}
    >
      <DebugContent parameters={list} ok={onOk} isNext={false}></DebugContent>
      {data && (
        <div className="mt-4 rounded-md bg-slate-200 border border-neutral-200">
          <div className="flex justify-between p-2">
            <span>JSON</span>
            <CopyToClipboard text={content}></CopyToClipboard>
          </div>
          <JsonView
            src={data}
            displaySize={30}
            className="w-full max-h-[300px] break-words overflow-auto p-2 bg-slate-100"
          />
        </div>
      )}
    </Drawer>
  );
};

export default SingleDebugDrawer;
