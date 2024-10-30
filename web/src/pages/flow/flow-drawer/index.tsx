import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { Drawer, Flex, Form, Input } from 'antd';
import { useEffect } from 'react';
import { Node } from 'reactflow';
import { Operator, operatorMap } from '../constant';
import AkShareForm from '../form/akshare-form';
import AnswerForm from '../form/answer-form';
import ArXivForm from '../form/arxiv-form';
import BaiduFanyiForm from '../form/baidu-fanyi-form';
import BaiduForm from '../form/baidu-form';
import BeginForm from '../form/begin-form';
import BingForm from '../form/bing-form';
import CategorizeForm from '../form/categorize-form';
import CrawlerForm from '../form/crawler-form';
import DeepLForm from '../form/deepl-form';
import DuckDuckGoForm from '../form/duckduckgo-form';
import ExeSQLForm from '../form/exesql-form';
import GenerateForm from '../form/generate-form';
import GithubForm from '../form/github-form';
import GoogleForm from '../form/google-form';
import GoogleScholarForm from '../form/google-scholar-form';
import InvokeForm from '../form/invoke-form';
import Jin10Form from '../form/jin10-form';
import KeywordExtractForm from '../form/keyword-extract-form';
import MessageForm from '../form/message-form';
import PubMedForm from '../form/pubmed-form';
import QWeatherForm from '../form/qweather-form';
import RelevantForm from '../form/relevant-form';
import RetrievalForm from '../form/retrieval-form';
import RewriteQuestionForm from '../form/rewrite-question-form';
import SwitchForm from '../form/switch-form';
import TuShareForm from '../form/tushare-form';
import WenCaiForm from '../form/wencai-form';
import WikipediaForm from '../form/wikipedia-form';
import YahooFinanceForm from '../form/yahoo-finance-form';
import { useHandleFormValuesChange, useHandleNodeNameChange } from '../hooks';
import OperatorIcon from '../operator-icon';

import { CloseOutlined } from '@ant-design/icons';
import { lowerFirst } from 'lodash';
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
  [Operator.GoogleScholar]: GoogleScholarForm,
  [Operator.DeepL]: DeepLForm,
  [Operator.GitHub]: GithubForm,
  [Operator.BaiduFanyi]: BaiduFanyiForm,
  [Operator.QWeather]: QWeatherForm,
  [Operator.ExeSQL]: ExeSQLForm,
  [Operator.Switch]: SwitchForm,
  [Operator.WenCai]: WenCaiForm,
  [Operator.AkShare]: AkShareForm,
  [Operator.YahooFinance]: YahooFinanceForm,
  [Operator.Jin10]: Jin10Form,
  [Operator.TuShare]: TuShareForm,
  [Operator.Crawler]: CrawlerForm,
  [Operator.Invoke]: InvokeForm,
  [Operator.Concentrator]: () => <></>,
  [Operator.Note]: () => <></>,
};

const EmptyContent = () => <div></div>;

const FlowDrawer = ({
  visible,
  hideModal,
  node,
}: IModalProps<any> & IProps) => {
  const operatorName: Operator = node?.data.label;
  const OperatorForm = FormMap[operatorName] ?? EmptyContent;
  const [form] = Form.useForm();
  const { name, handleNameBlur, handleNameChange } = useHandleNodeNameChange({
    id: node?.id,
    data: node?.data,
  });
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
        <Flex vertical>
          <Flex gap={'middle'} align="center">
            <OperatorIcon
              name={operatorName}
              color={operatorMap[operatorName]?.color}
            ></OperatorIcon>
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
            <CloseOutlined onClick={hideModal} />
          </Flex>
          <span className={styles.operatorDescription}>
            {t(`${lowerFirst(operatorName)}Description`)}
          </span>
        </Flex>
      }
      placement="right"
      onClose={hideModal}
      open={visible}
      getContainer={false}
      mask={false}
      width={500}
      closeIcon={null}
    >
      <section className={styles.formWrapper}>
        {visible && (
          <OperatorForm
            onValuesChange={handleValuesChange}
            form={form}
            node={node}
          ></OperatorForm>
        )}
      </section>
    </Drawer>
  );
};

export default FlowDrawer;
