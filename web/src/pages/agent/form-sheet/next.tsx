import { Input } from '@/components/ui/input';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { lowerFirst } from 'lodash';
import { Play, X } from 'lucide-react';
import { useMemo } from 'react';
import { BeginId, Operator } from '../constant';
import { AgentFormContext } from '../context';
import { RunTooltip } from '../flow-tooltip';
import { useHandleNodeNameChange } from '../hooks/use-change-node-name';
import OperatorIcon from '../operator-icon';
import useGraphStore from '../store';
import { needsSingleStepDebugging } from '../utils';
import { FormConfigMap } from './form-config-map';
import SingleDebugSheet from './single-debug-sheet';

interface IProps {
  node?: RAGFlowNodeType;
  singleDebugDrawerVisible: IModalProps<any>['visible'];
  hideSingleDebugDrawer: IModalProps<any>['hideModal'];
  showSingleDebugDrawer: IModalProps<any>['showModal'];
  chatVisible: boolean;
}

const EmptyContent = () => <div></div>;

const FormSheet = ({
  visible,
  hideModal,
  node,
  singleDebugDrawerVisible,
  chatVisible,
  hideSingleDebugDrawer,
  showSingleDebugDrawer,
}: IModalProps<any> & IProps) => {
  const operatorName: Operator = node?.data.label as Operator;
  const clickedToolId = useGraphStore((state) => state.clickedToolId);

  const currentFormMap = FormConfigMap[operatorName];

  const OperatorForm = currentFormMap?.component ?? EmptyContent;

  const { name, handleNameBlur, handleNameChange } = useHandleNodeNameChange({
    id: node?.id,
    data: node?.data,
  });

  const isMcp = useMemo(() => {
    return (
      operatorName === Operator.Tool &&
      Object.values(Operator).every((x) => x !== clickedToolId)
    );
  }, [clickedToolId, operatorName]);

  const { t } = useTranslate('flow');

  return (
    <Sheet open={visible} modal={false}>
      <SheetContent
        className={cn('top-20 p-0 flex flex-col pb-20 ', {
          'right-[620px]': chatVisible,
        })}
        closeIcon={false}
      >
        <SheetHeader>
          <SheetTitle className="hidden"></SheetTitle>
          <section className="flex-col border-b py-2 px-5">
            <div className="flex items-center gap-2 pb-3">
              <OperatorIcon name={operatorName}></OperatorIcon>

              {isMcp ? (
                <div className="flex-1">MCP Config</div>
              ) : (
                <div className="flex items-center gap-1 flex-1">
                  <label htmlFor="">{t('title')}</label>
                  {node?.id === BeginId ? (
                    <span>{t(BeginId)}</span>
                  ) : (
                    <Input
                      value={name}
                      onBlur={handleNameBlur}
                      onChange={handleNameChange}
                    ></Input>
                  )}
                </div>
              )}

              {needsSingleStepDebugging(operatorName) && (
                <RunTooltip>
                  <Play
                    className="size-5 cursor-pointer"
                    onClick={showSingleDebugDrawer}
                  />
                </RunTooltip>
              )}
              <X onClick={hideModal} />
            </div>
            {isMcp || (
              <span>
                {t(
                  `${lowerFirst(operatorName === Operator.Tool ? clickedToolId : operatorName)}Description`,
                )}
              </span>
            )}
          </section>
        </SheetHeader>
        <section className="pt-4 overflow-auto flex-1">
          {visible && (
            <AgentFormContext.Provider value={node}>
              <OperatorForm node={node} key={node?.id}></OperatorForm>
            </AgentFormContext.Provider>
          )}
        </section>
      </SheetContent>
      {singleDebugDrawerVisible && (
        <SingleDebugSheet
          visible={singleDebugDrawerVisible}
          hideModal={hideSingleDebugDrawer}
          componentId={node?.id}
        ></SingleDebugSheet>
      )}
    </Sheet>
  );
};

export default FormSheet;
