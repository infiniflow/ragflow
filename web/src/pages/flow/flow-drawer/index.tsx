import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { Drawer, Flex, Form, Input } from 'antd';
import { useEffect } from 'react';
import { Node } from 'reactflow';
import AnswerForm from '../answer-form';
import ArXivForm from '../arxiv-form';
import BaiduForm from '../baidu-form';
import BeginForm from '../begin-form';
import BingForm from '../bing-form';
import CategorizeForm from '../categorize-form';
import { Operator } from '../constant';
import DuckDuckGoForm from '../duckduckgo-form';
import GenerateForm from '../generate-form';
import GoogleForm from '../google-form';
import { useHandleFormValuesChange, useHandleNodeNameChange } from '../hooks';
import KeywordExtractForm from '../keyword-extract-form';
import MessageForm from '../message-form';
import OperatorIcon from '../operator-icon';
import PubMedForm from '../pubmed-form';
import RelevantForm from '../relevant-form';
import RetrievalForm from '../retrieval-form';
import RewriteQuestionForm from '../rewrite-question-form';
import WikipediaForm from '../wikipedia-form';

import styles from './index.less';

interface IProps {
  node?: Node;
}

const FormMap = {
  [Operator.Begin]: BeginForm,
  [Operator.Retrieval]: RetrievalForm,
  [Operator.Generate]: GenerateForm,
  [Operator.Answer]: AnswerForm,
  [Operator.Categorize]: CategorizeForm,
  [Operator.Message]: MessageForm,
  [Operator.Relevant]: RelevantForm,
  [Operator.RewriteQuestion]: RewriteQuestionForm,
  [Operator.Baidu]: BaiduForm,
  [Operator.DuckDuckGo]: DuckDuckGoForm,
  [Operator.KeywordExtract]: KeywordExtractForm,
  [Operator.Wikipedia]: WikipediaForm,
  [Operator.PubMed]: PubMedForm,
  [Operator.ArXiv]: ArXivForm,
  [Operator.Google]: GoogleForm,
  [Operator.Bing]: BingForm,
};

const EmptyContent = () => <div>empty</div>;

const FlowDrawer = ({
  visible,
  hideModal,
  node,
}: IModalProps<any> & IProps) => {
  const operatorName: Operator = node?.data.label;
  const OperatorForm = FormMap[operatorName] ?? EmptyContent;
  const [form] = Form.useForm();
  const { name, handleNameBlur, handleNameChange } =
    useHandleNodeNameChange(node);
  const { t } = useTranslate('flow');

  const { handleValuesChange } = useHandleFormValuesChange(node?.id);

  useEffect(() => {
    if (visible) {
      form.setFieldsValue(node?.data?.form);
    }
  }, [visible, form, node?.data?.form]);

  return (
    <Drawer
      title={
        <Flex gap={'middle'} align="center">
          <OperatorIcon name={operatorName}></OperatorIcon>
          <Flex align="center" gap={'small'} flex={1}>
            <label htmlFor="" className={styles.title}>
              {t('title')}
            </label>
            <Input
              value={name}
              onBlur={handleNameBlur}
              onChange={handleNameChange}
            ></Input>
          </Flex>
        </Flex>
      }
      placement="right"
      onClose={hideModal}
      open={visible}
      getContainer={false}
      mask={false}
      width={470}
    >
      {visible && (
        <OperatorForm
          onValuesChange={handleValuesChange}
          form={form}
          node={node}
        ></OperatorForm>
      )}
    </Drawer>
  );
};

export default FlowDrawer;
