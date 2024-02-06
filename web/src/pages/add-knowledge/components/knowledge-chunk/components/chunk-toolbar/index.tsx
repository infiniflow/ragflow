import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import {
  ArrowLeftOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  DeleteOutlined,
  DownOutlined,
  FilePdfOutlined,
  PlusOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { Button, Checkbox, Flex, Menu, MenuProps, Popover, Space } from 'antd';
import { useMemo } from 'react';
import { Link, useSelector } from 'umi';

const ChunkToolBar = () => {
  const documentInfo: IKnowledgeFile = useSelector(
    (state: any) => state.chunkModel.documentInfo,
  );

  const knowledgeBaseId = useKnowledgeBaseId();

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        label: (
          <>
            <Checkbox>
              <b>Select All</b>
            </Checkbox>
          </>
        ),
      },
      { type: 'divider' },
      {
        key: '2',
        label: (
          <Space>
            <CheckCircleOutlined />
            <b>Enabled Selected</b>
          </Space>
        ),
      },
      {
        key: '3',
        label: (
          <Space>
            <CloseCircleOutlined />
            <b>Disabled Selected</b>
          </Space>
        ),
      },
      { type: 'divider' },
      {
        key: '4',
        label: (
          <Space>
            <DeleteOutlined />
            <b>Delete Selected</b>
          </Space>
        ),
      },
    ];
  }, []);

  const content = (
    <Menu style={{ width: 200 }} items={items} selectable={false} />
  );

  return (
    <Flex justify="space-between" align="center">
      <Space size={'middle'}>
        <Link
          to={`/knowledge/${KnowledgeRouteKey.Dataset}?id=${knowledgeBaseId}`}
        >
          <ArrowLeftOutlined />
        </Link>
        <FilePdfOutlined />
        {documentInfo.name}
      </Space>
      <Space>
        <Popover content={content} placement="bottomLeft" arrow={false}>
          <Button>
            Bulk
            <DownOutlined />
          </Button>
        </Popover>
        <Button icon={<SearchOutlined />} />
        <Button icon={<FilterIcon />} />
        <Button icon={<DeleteOutlined />} />
        <Button icon={<PlusOutlined />} type="primary" />
      </Space>
    </Flex>
  );
};

export default ChunkToolBar;
