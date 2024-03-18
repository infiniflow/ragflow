import { ReactComponent as MoreModelIcon } from '@/assets/svg/more-model.svg';
import { useSetModalState } from '@/hooks/commonHooks';
import {
  LlmItem,
  useFetchLlmFactoryListOnMount,
  useFetchMyLlmListOnMount,
} from '@/hooks/llmHooks';
import { SettingOutlined } from '@ant-design/icons';
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
  Typography,
} from 'antd';
import { useCallback } from 'react';
import SettingTitle from '../components/setting-title';
import ApiKeyModal from './api-key-modal';
import { useSubmitApiKey, useSubmitSystemModelSetting } from './hooks';
import SystemModelSettingModal from './system-model-setting-modal';

import styles from './index.less';

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
              <Avatar shape="square" size="large" src={item.logo} />
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
            renderItem={(item) => <List.Item>{item.name}</List.Item>}
          />
        )}
      </Card>
    </List.Item>
  );
};

const UserSettingModel = () => {
  const factoryList = useFetchLlmFactoryListOnMount();
  const llmList = useFetchMyLlmListOnMount();
  const {
    saveApiKeyLoading,
    initialApiKey,
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
                  <Avatar shape="square" size="large" src={item.logo} />
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
      <ApiKeyModal
        visible={apiKeyVisible}
        hideModal={hideApiKeyModal}
        loading={saveApiKeyLoading}
        initialValue={initialApiKey}
        onOk={onApiKeySavingOk}
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
