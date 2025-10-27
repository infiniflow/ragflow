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
import { CirclePlay, X } from 'lucide-react';
import { Operator } from '../constant';
import { AgentFormContext } from '../context';
import { RunTooltip } from '../flow-tooltip';
import { useIsMcp } from '../hooks/use-is-mcp';
import OperatorIcon from '../operator-icon';
import useGraphStore from '../store';
import { needsSingleStepDebugging } from '../utils';
import { FormConfigMap } from './form-config-map';
import SingleDebugSheet from './single-debug-sheet';
import { TitleInput } from './title-input';

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

  const isMcp = useIsMcp(operatorName);

  const { t } = useTranslate('flow');

  return (
    <Sheet open={visible} modal={false}>
      <SheetContent
        className={cn('top-20 p-0 flex flex-col pb-20', {
          'right-[620px]': chatVisible,
        })}
        closeIcon={false}
      >
        <SheetHeader>
          <SheetTitle className="hidden"></SheetTitle>
          <section className="flex-col border-b py-2 px-5">
            <div className="flex items-center gap-2 pb-3">
              <OperatorIcon name={operatorName}></OperatorIcon>
              <TitleInput node={node}></TitleInput>
              {needsSingleStepDebugging(operatorName) && (
                <RunTooltip>
                  <CirclePlay
                    className="size-3.5 cursor-pointer"
                    onClick={showSingleDebugDrawer}
                  />
                </RunTooltip>
              )}
              <X onClick={hideModal} className="size-3.5 cursor-pointer" />
            </div>
            {isMcp || (
              <span className="text-text-secondary">
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
