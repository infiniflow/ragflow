import { ReactComponent as MoreModelIcon } from '@/assets/svg/more-model.svg';
import { useSetModalState } from '@/hooks/commonHooks';
import {
  LlmItem,
  useFetchLlmFactoryListOnMount,
  useFetchMyLlmListOnMount,
} from '@/hooks/llmHooks';
import { ReactComponent as MoonshotIcon } from '@/icons/moonshot.svg';
import { ReactComponent as OpenAiIcon } from '@/icons/openai.svg';
import { ReactComponent as TongYiIcon } from '@/icons/tongyi.svg';
import { ReactComponent as WenXinIcon } from '@/icons/wenxin.svg';
import { ReactComponent as ZhiPuIcon } from '@/icons/zhipu.svg';
import { SettingOutlined, UserOutlined } from '@ant-design/icons';
import {
  Avatar,
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
  Typography,
} from 'antd';
import { useCallback } from 'react';
import SettingTitle from '../components/setting-title';
import ApiKeyModal from './api-key-modal';
import {
  useSelectModelProvidersLoading,
  useSubmitApiKey,
  useSubmitSystemModelSetting,
} from './hooks';
import SystemModelSettingModal from './system-model-setting-modal';

import styles from './index.less';

const IconMap = {
  'Tongyi-Qianwen': TongYiIcon,
  Moonshot: MoonshotIcon,
  OpenAI: OpenAiIcon,
  'ZHIPU-AI': ZhiPuIcon,
  文心一言: WenXinIcon,
};

const LlmIcon = ({ name }: { name: string }) => {
  const Icon = IconMap[name as keyof typeof IconMap];
  return Icon ? (
    <Icon width={48} height={48}></Icon>
  ) : (
    <Avatar shape="square" size="large" icon={<UserOutlined />} />
  );
};

const { Text } = Typography;
interface IModelCardProps {
  item: LlmItem;
  clickApiKey: (llmFactory: string) => void;
}

const ModelCard = ({ item, clickApiKey }: IModelCardProps) => {
  const { visible, switchVisible } = useSetModalState();

  const handleApiKeyClick = () => {
    clickApiKey(item.name);
  };

  const handleShowMoreClick = () => {
    switchVisible();
  };

  return (
    <List.Item>
      <Card className={styles.addedCard}>
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
                API-Key
                <SettingOutlined />
              </Button>
              <Button onClick={handleShowMoreClick}>
                <Flex gap={'small'}>
                  Show more models
                  <MoreModelIcon />
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
                  {item.name} <Tag color="#b8b8b8">{item.type}</Tag>
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
  const factoryList = useFetchLlmFactoryListOnMount();
  const llmList = useFetchMyLlmListOnMount();
  const loading = useSelectModelProvidersLoading();
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

  const handleApiKeyClick = useCallback(
    (llmFactory: string) => {
      showApiKeyModal({ llm_factory: llmFactory });
    },
    [showApiKeyModal],
  );

  const handleAddModel = (llmFactory: string) => () => {
    handleApiKeyClick(llmFactory);
  };

  const items: CollapseProps['items'] = [
    {
      key: '1',
      label: 'Added models',
      children: (
        <List
          grid={{ gutter: 16, column: 1 }}
          dataSource={llmList}
          renderItem={(item) => (
            <ModelCard item={item} clickApiKey={handleApiKeyClick}></ModelCard>
          )}
        />
      ),
    },
    {
      key: '2',
      label: 'Models to be added',
      children: (
        <List
          grid={{
            gutter: 24,
            xs: 1,
            sm: 2,
            md: 3,
            lg: 4,
            xl: 4,
            xxl: 8,
          }}
          dataSource={factoryList}
          renderItem={(item) => (
            <List.Item>
              <Card className={styles.toBeAddedCard}>
                <Flex vertical gap={'large'}>
                  <LlmIcon name={item.name} />
                  <Flex vertical gap={'middle'}>
                    <b>{item.name}</b>
                    <Text>{item.tags}</Text>
                  </Flex>
                </Flex>
                <Divider></Divider>
                <Button type="link" onClick={handleAddModel(item.name)}>
                  Add the model
                </Button>
              </Card>
            </List.Item>
          )}
        />
      ),
    },
  ];

  return (
    <>
      <Spin spinning={loading}>
        <section className={styles.modelWrapper}>
          <SettingTitle
            title="Model Setting"
            description="Manage your account settings and preferences here."
            showRightButton
            clickButton={showSystemSettingModal}
          ></SettingTitle>
          <Divider></Divider>
          <Collapse defaultActiveKey={['1']} ghost items={items} />
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
      <SystemModelSettingModal
        visible={systemSettingVisible}
        onOk={onSystemSettingSavingOk}
        hideModal={hideSystemSettingModal}
        loading={saveSystemModelSettingLoading}
      ></SystemModelSettingModal>
    </>
  );
};

export default UserSettingModel;
