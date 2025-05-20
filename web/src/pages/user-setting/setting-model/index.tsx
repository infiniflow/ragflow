import { ReactComponent as MoreModelIcon } from '@/assets/svg/more-model.svg';
import { LlmIcon } from '@/components/svg-icon';
import { useTheme } from '@/components/theme-provider';
import { LLMFactory } from '@/constants/llm';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { LlmItem, useSelectLlmList } from '@/hooks/llm-hooks';
import { getRealModelName } from '@/utils/llm-util';
import { CloseCircleOutlined, SettingOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  Collapse,
  CollapseProps,
  Divider,
  Flex,
  List,
  Row,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import { CircleHelp } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import SettingTitle from '../components/setting-title';
import { isLocalLlmFactory } from '../utils';
import TencentCloudModal from './Tencent-modal';
import ApiKeyModal from './api-key-modal';
import AzureOpenAIModal from './azure-openai-modal';
import BedrockModal from './bedrock-modal';
import FishAudioModal from './fish-audio-modal';
import GoogleModal from './google-modal';
import {
  useHandleDeleteFactory,
  useHandleDeleteLlm,
  useSubmitApiKey,
  useSubmitAzure,
  useSubmitBedrock,
  useSubmitFishAudio,
  useSubmitGoogle,
  useSubmitHunyuan,
  useSubmitOllama,
  useSubmitSpark,
  useSubmitSystemModelSetting,
  useSubmitTencentCloud,
  useSubmitVolcEngine,
  useSubmityiyan,
} from './hooks';
import HunyuanModal from './hunyuan-modal';
import styles from './index.less';
import OllamaModal from './ollama-modal';
import SparkModal from './spark-modal';
import SystemModelSettingModal from './system-model-setting-modal';
import VolcEngineModal from './volcengine-modal';
import YiyanModal from './yiyan-modal';

const { Text } = Typography;
interface IModelCardProps {
  item: LlmItem;
  clickApiKey: (llmFactory: string) => void;
}

const ModelCard = ({ item, clickApiKey }: IModelCardProps) => {
  const { visible, switchVisible } = useSetModalState();
  const { t } = useTranslate('setting');
  const { theme } = useTheme();
  const { handleDeleteLlm } = useHandleDeleteLlm(item.name);
  const { handleDeleteFactory } = useHandleDeleteFactory(item.name);

  const handleApiKeyClick = () => {
    clickApiKey(item.name);
  };

  const handleShowMoreClick = () => {
    switchVisible();
  };

  return (
    <List.Item>
      <Card
        className={theme === 'dark' ? styles.addedCardDark : styles.addedCard}
      >
        <Row align={'middle'}>
          <Col span={12}>
            <Flex gap={'middle'} align="center">
              <LlmIcon name={item.name} />
              <Flex vertical gap={'small'}>
                <b>{item.name}</b>
                <Text>{item.tags}</Text>
              </Flex>
            </Flex>
          </Col>
          <Col span={12} className={styles.factoryOperationWrapper}>
            <Space size={'middle'}>
              <Button onClick={handleApiKeyClick}>
                <Flex align="center" gap={4}>
                  {isLocalLlmFactory(item.name) ||
                  item.name === LLMFactory.VolcEngine ||
                  item.name === LLMFactory.TencentHunYuan ||
                  item.name === LLMFactory.XunFeiSpark ||
                  item.name === LLMFactory.BaiduYiYan ||
                  item.name === LLMFactory.FishAudio ||
                  item.name === LLMFactory.TencentCloud ||
                  item.name === LLMFactory.GoogleCloud ||
                  item.name === LLMFactory.AzureOpenAI
                    ? t('addTheModel')
                    : 'API-Key'}
                  <SettingOutlined />
                </Flex>
              </Button>
              <Button onClick={handleShowMoreClick}>
                <Flex align="center" gap={4}>
                  {t('showMoreModels')}
                  <MoreModelIcon />
                </Flex>
              </Button>
              <Button type={'text'} onClick={handleDeleteFactory}>
                <Flex align="center">
                  <CloseCircleOutlined style={{ color: '#D92D20' }} />
                </Flex>
              </Button>
            </Space>
          </Col>
        </Row>
        {visible && (
          <List
            size="small"
            dataSource={item.llm}
            className={styles.llmList}
            renderItem={(item) => (
              <List.Item>
                <Space>
                  {getRealModelName(item.name)}
                  <Tag color="#b8b8b8">{item.type}</Tag>
                  <Tooltip title={t('delete', { keyPrefix: 'common' })}>
                    <Button type={'text'} onClick={handleDeleteLlm(item.name)}>
                      <CloseCircleOutlined style={{ color: '#D92D20' }} />
                    </Button>
                  </Tooltip>
                </Space>
              </List.Item>
            )}
          />
        )}
      </Card>
    </List.Item>
  );
};

const UserSettingModel = () => {
  const { factoryList, myLlmList: llmList, loading } = useSelectLlmList();
  const { theme } = useTheme();
  const {
    saveApiKeyLoading,
    initialApiKey,
    llmFactory,
    onApiKeySavingOk,
    apiKeyVisible,
    hideApiKeyModal,
    showApiKeyModal,
  } = useSubmitApiKey();
  const {
    saveSystemModelSettingLoading,
    onSystemSettingSavingOk,
    systemSettingVisible,
    hideSystemSettingModal,
    showSystemSettingModal,
  } = useSubmitSystemModelSetting();
  const { t } = useTranslate('setting');
  const {
    llmAddingVisible,
    hideLlmAddingModal,
    showLlmAddingModal,
    onLlmAddingOk,
    llmAddingLoading,
    selectedLlmFactory,
  } = useSubmitOllama();

  const {
    volcAddingVisible,
    hideVolcAddingModal,
    showVolcAddingModal,
    onVolcAddingOk,
    volcAddingLoading,
  } = useSubmitVolcEngine();

  const {
    HunyuanAddingVisible,
    hideHunyuanAddingModal,
    showHunyuanAddingModal,
    onHunyuanAddingOk,
    HunyuanAddingLoading,
  } = useSubmitHunyuan();

  const {
    GoogleAddingVisible,
    hideGoogleAddingModal,
    showGoogleAddingModal,
    onGoogleAddingOk,
    GoogleAddingLoading,
  } = useSubmitGoogle();

  const {
    TencentCloudAddingVisible,
    hideTencentCloudAddingModal,
    showTencentCloudAddingModal,
    onTencentCloudAddingOk,
    TencentCloudAddingLoading,
  } = useSubmitTencentCloud();

  const {
    SparkAddingVisible,
    hideSparkAddingModal,
    showSparkAddingModal,
    onSparkAddingOk,
    SparkAddingLoading,
  } = useSubmitSpark();

  const {
    yiyanAddingVisible,
    hideyiyanAddingModal,
    showyiyanAddingModal,
    onyiyanAddingOk,
    yiyanAddingLoading,
  } = useSubmityiyan();

  const {
    FishAudioAddingVisible,
    hideFishAudioAddingModal,
    showFishAudioAddingModal,
    onFishAudioAddingOk,
    FishAudioAddingLoading,
  } = useSubmitFishAudio();

  const {
    bedrockAddingLoading,
    onBedrockAddingOk,
    bedrockAddingVisible,
    hideBedrockAddingModal,
    showBedrockAddingModal,
  } = useSubmitBedrock();

  const {
    AzureAddingVisible,
    hideAzureAddingModal,
    showAzureAddingModal,
    onAzureAddingOk,
    AzureAddingLoading,
  } = useSubmitAzure();

  const ModalMap = useMemo(
    () => ({
      [LLMFactory.Bedrock]: showBedrockAddingModal,
      [LLMFactory.VolcEngine]: showVolcAddingModal,
      [LLMFactory.TencentHunYuan]: showHunyuanAddingModal,
      [LLMFactory.XunFeiSpark]: showSparkAddingModal,
      [LLMFactory.BaiduYiYan]: showyiyanAddingModal,
      [LLMFactory.FishAudio]: showFishAudioAddingModal,
      [LLMFactory.TencentCloud]: showTencentCloudAddingModal,
      [LLMFactory.GoogleCloud]: showGoogleAddingModal,
      [LLMFactory.AzureOpenAI]: showAzureAddingModal,
    }),
    [
      showBedrockAddingModal,
      showVolcAddingModal,
      showHunyuanAddingModal,
      showTencentCloudAddingModal,
      showSparkAddingModal,
      showyiyanAddingModal,
      showFishAudioAddingModal,
      showGoogleAddingModal,
      showAzureAddingModal,
    ],
  );

  const handleAddModel = useCallback(
    (llmFactory: string) => {
      if (isLocalLlmFactory(llmFactory)) {
        showLlmAddingModal(llmFactory);
      } else if (llmFactory in ModalMap) {
        ModalMap[llmFactory as keyof typeof ModalMap]();
      } else {
        showApiKeyModal({ llm_factory: llmFactory });
      }
    },
    [showApiKeyModal, showLlmAddingModal, ModalMap],
  );

  const items: CollapseProps['items'] = [
    {
      key: '1',
      label: t('addedModels'),
      children: (
        <List
          grid={{ gutter: 16, column: 1 }}
          dataSource={llmList}
          renderItem={(item) => (
            <ModelCard item={item} clickApiKey={handleAddModel}></ModelCard>
          )}
        />
      ),
    },
    {
      key: '2',
      label: (
        <div className="flex items-center gap-2">
          {t('modelsToBeAdded')}
          <Tooltip title={t('modelsToBeAddedTooltip')}>
            <CircleHelp className="size-4" />
          </Tooltip>
        </div>
      ),
      children: (
        <List
          grid={{
            gutter: {
              xs: 8,
              sm: 10,
              md: 12,
              lg: 16,
              xl: 20,
              xxl: 24,
            },
            xs: 1,
            sm: 1,
            md: 2,
            lg: 3,
            xl: 4,
            xxl: 8,
          }}
          dataSource={factoryList}
          renderItem={(item) => (
            <List.Item>
              <Card
                className={
                  theme === 'dark'
                    ? styles.toBeAddedCardDark
                    : styles.toBeAddedCard
                }
              >
                <Flex vertical gap={'middle'}>
                  <LlmIcon name={item.name} imgClass="h-12 w-auto" />
                  <Flex vertical gap={'middle'}>
                    <b>
                      <Text ellipsis={{ tooltip: item.name }}>{item.name}</Text>
                    </b>
                    <Text className={styles.modelTags}>{item.tags}</Text>
                  </Flex>
                </Flex>
                <Divider className={styles.modelDivider}></Divider>
                <Button
                  type="link"
                  onClick={() => handleAddModel(item.name)}
                  className={styles.addButton}
                >
                  {t('addTheModel')}
                </Button>
              </Card>
            </List.Item>
          )}
        />
      ),
    },
  ];

  return (
    <section id="xx" className="w-full space-y-6">
      <Spin spinning={loading}>
        <section className={styles.modelContainer}>
          <SettingTitle
            title={t('model')}
            description={t('modelDescription')}
            showRightButton
            clickButton={showSystemSettingModal}
          ></SettingTitle>
          <Divider></Divider>
          <Collapse defaultActiveKey={['1', '2']} ghost items={items} />
        </section>
      </Spin>
      <ApiKeyModal
        visible={apiKeyVisible}
        hideModal={hideApiKeyModal}
        loading={saveApiKeyLoading}
        initialValue={initialApiKey}
        onOk={onApiKeySavingOk}
        llmFactory={llmFactory}
      ></ApiKeyModal>
      {systemSettingVisible && (
        <SystemModelSettingModal
          visible={systemSettingVisible}
          onOk={onSystemSettingSavingOk}
          hideModal={hideSystemSettingModal}
          loading={saveSystemModelSettingLoading}
        ></SystemModelSettingModal>
      )}
      <OllamaModal
        visible={llmAddingVisible}
        hideModal={hideLlmAddingModal}
        onOk={onLlmAddingOk}
        loading={llmAddingLoading}
        llmFactory={selectedLlmFactory}
      ></OllamaModal>
      <VolcEngineModal
        visible={volcAddingVisible}
        hideModal={hideVolcAddingModal}
        onOk={onVolcAddingOk}
        loading={volcAddingLoading}
        llmFactory={LLMFactory.VolcEngine}
      ></VolcEngineModal>
      <HunyuanModal
        visible={HunyuanAddingVisible}
        hideModal={hideHunyuanAddingModal}
        onOk={onHunyuanAddingOk}
        loading={HunyuanAddingLoading}
        llmFactory={LLMFactory.TencentHunYuan}
      ></HunyuanModal>
      <GoogleModal
        visible={GoogleAddingVisible}
        hideModal={hideGoogleAddingModal}
        onOk={onGoogleAddingOk}
        loading={GoogleAddingLoading}
        llmFactory={LLMFactory.GoogleCloud}
      ></GoogleModal>
      <TencentCloudModal
        visible={TencentCloudAddingVisible}
        hideModal={hideTencentCloudAddingModal}
        onOk={onTencentCloudAddingOk}
        loading={TencentCloudAddingLoading}
        llmFactory={LLMFactory.TencentCloud}
      ></TencentCloudModal>
      <SparkModal
        visible={SparkAddingVisible}
        hideModal={hideSparkAddingModal}
        onOk={onSparkAddingOk}
        loading={SparkAddingLoading}
        llmFactory={LLMFactory.XunFeiSpark}
      ></SparkModal>
      <YiyanModal
        visible={yiyanAddingVisible}
        hideModal={hideyiyanAddingModal}
        onOk={onyiyanAddingOk}
        loading={yiyanAddingLoading}
        llmFactory={LLMFactory.BaiduYiYan}
      ></YiyanModal>
      <FishAudioModal
        visible={FishAudioAddingVisible}
        hideModal={hideFishAudioAddingModal}
        onOk={onFishAudioAddingOk}
        loading={FishAudioAddingLoading}
        llmFactory={LLMFactory.FishAudio}
      ></FishAudioModal>
      <BedrockModal
        visible={bedrockAddingVisible}
        hideModal={hideBedrockAddingModal}
        onOk={onBedrockAddingOk}
        loading={bedrockAddingLoading}
        llmFactory={LLMFactory.Bedrock}
      ></BedrockModal>
      <AzureOpenAIModal
        visible={AzureAddingVisible}
        hideModal={hideAzureAddingModal}
        onOk={onAzureAddingOk}
        loading={AzureAddingLoading}
        llmFactory={LLMFactory.AzureOpenAI}
      ></AzureOpenAIModal>
    </section>
  );
};

export default UserSettingModel;
