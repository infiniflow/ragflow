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
import { zodResolver } from '@hookform/resolvers/zod';
import { get, isPlainObject, lowerFirst } from 'lodash';
import { Play, X } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useForm } from 'react-hook-form';
import { BeginId, Operator, operatorMap } from '../constant';
import { FlowFormContext } from '../context';
import { RunTooltip } from '../flow-tooltip';
import { useHandleFormValuesChange, useHandleNodeNameChange } from '../hooks';
import OperatorIcon from '../operator-icon';
import {
  buildCategorizeListFromObject,
  needsSingleStepDebugging,
} from '../utils';
import SingleDebugDrawer from './single-debug-drawer';
import { useFormConfigMap } from './use-form-config-map';

interface IProps {
  node?: RAGFlowNodeType;
  singleDebugDrawerVisible: IModalProps<any>['visible'];
  hideSingleDebugDrawer: IModalProps<any>['hideModal'];
  showSingleDebugDrawer: IModalProps<any>['showModal'];
}

const EmptyContent = () => <div></div>;

const FormSheet = ({
  visible,
  hideModal,
  node,
  singleDebugDrawerVisible,
  hideSingleDebugDrawer,
  showSingleDebugDrawer,
}: IModalProps<any> & IProps) => {
  const operatorName: Operator = node?.data.label as Operator;

  const FormConfigMap = useFormConfigMap();

  const currentFormMap = FormConfigMap[operatorName];

  const OperatorForm = currentFormMap.component ?? EmptyContent;

  const form = useForm({
    defaultValues: currentFormMap.defaultValues,
    resolver: zodResolver(currentFormMap.schema),
  });

  const { name, handleNameBlur, handleNameChange } = useHandleNodeNameChange({
    id: node?.id,
    data: node?.data,
  });

  const previousId = useRef<string | undefined>(node?.id);

  const { t } = useTranslate('flow');

  const { handleValuesChange } = useHandleFormValuesChange(
    operatorName,
    node?.id,
    form,
  );

  useEffect(() => {
    if (visible && !form.formState.isDirty) {
      if (node?.id !== previousId.current) {
        form.reset();
        form.clearErrors();
      }

      if (operatorName === Operator.Categorize) {
        const items = buildCategorizeListFromObject(
          get(node, 'data.form.category_description', {}),
        );
        const formData = node?.data?.form;
        if (isPlainObject(formData)) {
          //   form.setFieldsValue({ ...formData, items });
          form.reset({ ...formData, items });
        }
      } else {
        // form.setFieldsValue(node?.data?.form);
        form.reset(node?.data?.form);
      }
      previousId.current = node?.id;
    }
  }, [visible, form, node?.data?.form, node?.id, node, operatorName]);

  return (
    <Sheet open={visible} modal={false}>
      <SheetTitle className="hidden"></SheetTitle>
      <SheetContent className={cn('bg-white top-20 p-0')} closeIcon={false}>
        <SheetHeader>
          <section className="flex-col border-b py-2 px-5">
            <div className="flex items-center gap-2 pb-3">
              <OperatorIcon
                name={operatorName}
                color={operatorMap[operatorName]?.color}
              ></OperatorIcon>
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
            <span>{t(`${lowerFirst(operatorName)}Description`)}</span>
          </section>
        </SheetHeader>
        <section className="pt-4">
          {visible && (
            <FlowFormContext.Provider value={node}>
              <OperatorForm
                onValuesChange={handleValuesChange}
                form={form}
                node={node}
              ></OperatorForm>
            </FlowFormContext.Provider>
          )}
        </section>
      </SheetContent>
      {singleDebugDrawerVisible && (
        <SingleDebugDrawer
          visible={singleDebugDrawerVisible}
          hideModal={hideSingleDebugDrawer}
          componentId={node?.id}
        ></SingleDebugDrawer>
      )}
    </Sheet>
  );
};

export default FormSheet;
