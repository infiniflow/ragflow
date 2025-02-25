import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { CloseOutlined } from '@ant-design/icons';
import { Flex, Input } from 'antd';
import { get, isPlainObject, lowerFirst } from 'lodash';
import { Play } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { BeginId, Operator, operatorMap } from '../constant';
import { useHandleFormValuesChange, useHandleNodeNameChange } from '../hooks';
import OperatorIcon from '../operator-icon';
import {
  buildCategorizeListFromObject,
  needsSingleStepDebugging,
} from '../utils';
import SingleDebugDrawer from './single-debug-drawer';

import { Sheet, SheetContent, SheetHeader } from '@/components/ui/sheet';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { FlowFormContext } from '../context';
import { RunTooltip } from '../flow-tooltip';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useFormConfigMap } from './use-form-config-map';

interface IProps {
  node?: RAGFlowNodeType;
  singleDebugDrawerVisible: IModalProps<any>['visible'];
  hideSingleDebugDrawer: IModalProps<any>['hideModal'];
  showSingleDebugDrawer: IModalProps<any>['showModal'];
}

const EmptyContent = () => <div></div>;

const FormDrawer = ({
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

  const { handleValuesChange } = useHandleFormValuesChange(node?.id);

  useEffect(() => {
    if (visible) {
      if (node?.id !== previousId.current) {
        // form.resetFields();
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
    <Sheet onOpenChange={hideModal} open={visible}>
      <SheetContent className="bg-white">
        <SheetHeader>
          <Flex vertical>
            <Flex gap={'middle'} align="center">
              <OperatorIcon
                name={operatorName}
                color={operatorMap[operatorName]?.color}
              ></OperatorIcon>
              <Flex align="center" gap={'small'} flex={1}>
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
              </Flex>

              {needsSingleStepDebugging(operatorName) && (
                <RunTooltip>
                  <Play
                    className="size-5 cursor-pointer"
                    onClick={showSingleDebugDrawer}
                  />
                </RunTooltip>
              )}
              <CloseOutlined onClick={hideModal} />
            </Flex>
            <span>{t(`${lowerFirst(operatorName)}Description`)}</span>
          </Flex>
        </SheetHeader>
        <section>
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

export default FormDrawer;
