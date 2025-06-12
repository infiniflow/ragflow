import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { CloseOutlined } from '@ant-design/icons';
import { Drawer, Flex, Form, Input } from 'antd';
import { get, isPlainObject, lowerFirst } from 'lodash';
import { Play } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { BeginId, Operator, operatorMap } from '../constant';
import AkShareForm from '../form/akshare-form';
import AnswerForm from '../form/answer-form';
import ArXivForm from '../form/arxiv-form';
import BaiduFanyiForm from '../form/baidu-fanyi-form';
import BaiduForm from '../form/baidu-form';
import BeginForm from '../form/begin-form';
import BingForm from '../form/bing-form';
import CategorizeForm from '../form/categorize-form';
import CodeForm from '../form/code-form';
import CrawlerForm from '../form/crawler-form';
import DeepLForm from '../form/deepl-form';
import DuckDuckGoForm from '../form/duckduckgo-form';
import EmailForm from '../form/email-form';
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
import TemplateForm from '../form/template-form';
import TuShareForm from '../form/tushare-form';
import WenCaiForm from '../form/wencai-form';
import WikipediaForm from '../form/wikipedia-form';
import YahooFinanceForm from '../form/yahoo-finance-form';
import { useHandleFormValuesChange, useHandleNodeNameChange } from '../hooks';
import OperatorIcon from '../operator-icon';
import {
  buildCategorizeListFromObject,
  getDrawerWidth,
  needsSingleStepDebugging,
} from '../utils';
import SingleDebugDrawer from './single-debug-drawer';

import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { FlowFormContext } from '../context';
import { RunTooltip } from '../flow-tooltip';
import IterationForm from '../form/iteration-from';
import styles from './index.less';

interface IProps {
  node?: RAGFlowNodeType;
  singleDebugDrawerVisible: IModalProps<any>['visible'];
  hideSingleDebugDrawer: IModalProps<any>['hideModal'];
  showSingleDebugDrawer: IModalProps<any>['showModal'];
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
  [Operator.Template]: TemplateForm,
  [Operator.Email]: EmailForm,
  [Operator.Iteration]: IterationForm,
  [Operator.IterationStart]: () => <></>,
  [Operator.Code]: CodeForm,
};

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
  const OperatorForm = FormMap[operatorName] ?? EmptyContent;
  const [form] = Form.useForm();
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
        form.resetFields();
      }

      if (operatorName === Operator.Categorize) {
        const items = buildCategorizeListFromObject(
          get(node, 'data.form.category_description', {}),
        );
        const formData = node?.data?.form;
        if (isPlainObject(formData)) {
          form.setFieldsValue({ ...formData, items });
        }
      } else {
        form.setFieldsValue(node?.data?.form);
      }
      previousId.current = node?.id;
    }
  }, [visible, form, node?.data?.form, node?.id, node, operatorName]);

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
      width={getDrawerWidth()}
      closeIcon={null}
      rootClassName={styles.formDrawer}
    >
      <section className={styles.formWrapper}>
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
      {singleDebugDrawerVisible && (
        <SingleDebugDrawer
          visible={singleDebugDrawerVisible}
          hideModal={hideSingleDebugDrawer}
          componentId={node?.id}
        ></SingleDebugDrawer>
      )}
    </Drawer>
  );
};

export default FormDrawer;
