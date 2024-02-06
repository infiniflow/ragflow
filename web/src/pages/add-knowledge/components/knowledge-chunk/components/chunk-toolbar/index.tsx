import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
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
import {
  Button,
  Checkbox,
  Flex,
  Menu,
  MenuProps,
  Popover,
  Radio,
  RadioChangeEvent,
  Space,
} from 'antd';
import { useCallback, useMemo } from 'react';
import { Link, useDispatch, useSelector } from 'umi';
import { ChunkModelState } from '../../model';

interface IProps {
  checked: boolean;
  getChunkList: () => void;
  selectAllChunk: (checked: boolean) => void;
}

const ChunkToolBar = ({ getChunkList, selectAllChunk, checked }: IProps) => {
  const { documentInfo, available }: ChunkModelState = useSelector(
    (state: any) => state.chunkModel,
  );
  const dispatch = useDispatch();

  const knowledgeBaseId = useKnowledgeBaseId();

  const handleSelectAllCheck = useCallback(
    (e: any) => {
      // console.info(e.target.checked);
      selectAllChunk(e.target.checked);
    },
    [selectAllChunk],
  );

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        label: (
          <>
            <Checkbox onChange={handleSelectAllCheck} checked={checked}>
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
  }, [checked, handleSelectAllCheck]);

  const content = (
    <Menu style={{ width: 200 }} items={items} selectable={false} />
  );

  const handleFilterChange = (e: RadioChangeEvent) => {
    dispatch({ type: 'chunkModel/setAvailable', payload: e.target.value });
    getChunkList();
  };

  const filterContent = (
    <Radio.Group onChange={handleFilterChange} value={available}>
      <Space direction="vertical">
        <Radio value={undefined}>All</Radio>
        <Radio value={1}>Enabled</Radio>
        <Radio value={0}>Disabled</Radio>
      </Space>
    </Radio.Group>
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
        <Popover content={content} placement="bottom" arrow={false}>
          <Button>
            Bulk
            <DownOutlined />
          </Button>
        </Popover>
        <Button icon={<SearchOutlined />} />
        <Popover content={filterContent} placement="bottom" arrow={false}>
          <Button icon={<FilterIcon />} />
        </Popover>
        <Button icon={<DeleteOutlined />} />
        <Button icon={<PlusOutlined />} type="primary" />
      </Space>
    </Flex>
  );
};

export default ChunkToolBar;
