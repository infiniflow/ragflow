import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { IChunkListResult, useSelectChunkList } from '@/hooks/chunk-hooks';
import { useTranslate } from '@/hooks/common-hooks';
import { useKnowledgeBaseId } from '@/hooks/knowledge-hooks';
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
  Input,
  Menu,
  MenuProps,
  Popover,
  Radio,
  RadioChangeEvent,
  Segmented,
  SegmentedProps,
  Space,
  Typography,
} from 'antd';
import { useCallback, useMemo, useState } from 'react';
import { Link } from 'umi';
import { ChunkTextMode } from '../../constant';

const { Text } = Typography;

interface IProps
  extends Pick<
    IChunkListResult,
    'searchString' | 'handleInputChange' | 'available' | 'handleSetAvailable'
  > {
  checked: boolean;
  selectAllChunk: (checked: boolean) => void;
  createChunk: () => void;
  removeChunk: () => void;
  switchChunk: (available: number) => void;
  changeChunkTextMode(mode: ChunkTextMode): void;
}

const ChunkToolBar = ({
  selectAllChunk,
  checked,
  createChunk,
  removeChunk,
  switchChunk,
  changeChunkTextMode,
  available,
  handleSetAvailable,
  searchString,
  handleInputChange,
}: IProps) => {
  const data = useSelectChunkList();
  const documentInfo = data?.documentInfo;
  const knowledgeBaseId = useKnowledgeBaseId();
  const [isShowSearchBox, setIsShowSearchBox] = useState(false);
  const { t } = useTranslate('chunk');

  const handleSelectAllCheck = useCallback(
    (e: any) => {
      selectAllChunk(e.target.checked);
    },
    [selectAllChunk],
  );

  const handleSearchIconClick = () => {
    setIsShowSearchBox(true);
  };

  const handleSearchBlur = () => {
    if (!searchString?.trim()) {
      setIsShowSearchBox(false);
    }
  };

  const handleDelete = useCallback(() => {
    removeChunk();
  }, [removeChunk]);

  const handleEnabledClick = useCallback(() => {
    switchChunk(1);
  }, [switchChunk]);

  const handleDisabledClick = useCallback(() => {
    switchChunk(0);
  }, [switchChunk]);

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        label: (
          <>
            <Checkbox onChange={handleSelectAllCheck} checked={checked}>
              <b>{t('selectAll')}</b>
            </Checkbox>
          </>
        ),
      },
      { type: 'divider' },
      {
        key: '2',
        label: (
          <Space onClick={handleEnabledClick}>
            <CheckCircleOutlined />
            <b>{t('enabledSelected')}</b>
          </Space>
        ),
      },
      {
        key: '3',
        label: (
          <Space onClick={handleDisabledClick}>
            <CloseCircleOutlined />
            <b>{t('disabledSelected')}</b>
          </Space>
        ),
      },
      { type: 'divider' },
      {
        key: '4',
        label: (
          <Space onClick={handleDelete}>
            <DeleteOutlined />
            <b>{t('deleteSelected')}</b>
          </Space>
        ),
      },
    ];
  }, [
    checked,
    handleSelectAllCheck,
    handleDelete,
    handleEnabledClick,
    handleDisabledClick,
    t,
  ]);

  const content = (
    <Menu style={{ width: 200 }} items={items} selectable={false} />
  );

  const handleFilterChange = (e: RadioChangeEvent) => {
    selectAllChunk(false);
    handleSetAvailable(e.target.value);
  };

  const filterContent = (
    <Radio.Group onChange={handleFilterChange} value={available}>
      <Space direction="vertical">
        <Radio value={undefined}>{t('all')}</Radio>
        <Radio value={1}>{t('enabled')}</Radio>
        <Radio value={0}>{t('disabled')}</Radio>
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
        <Text ellipsis={{ tooltip: documentInfo?.name }} style={{ width: 150 }}>
          {documentInfo?.name}
        </Text>
      </Space>
      <Space>
        <Segmented
          options={[
            { label: t(ChunkTextMode.Full), value: ChunkTextMode.Full },
            { label: t(ChunkTextMode.Ellipse), value: ChunkTextMode.Ellipse },
          ]}
          onChange={changeChunkTextMode as SegmentedProps['onChange']}
        />
        <Popover content={content} placement="bottom" arrow={false}>
          <Button>
            {t('bulk')}
            <DownOutlined />
          </Button>
        </Popover>
        {isShowSearchBox ? (
          <Input
            size="middle"
            placeholder={t('search')}
            prefix={<SearchOutlined />}
            allowClear
            onChange={handleInputChange}
            onBlur={handleSearchBlur}
            value={searchString}
          />
        ) : (
          <Button icon={<SearchOutlined />} onClick={handleSearchIconClick} />
        )}

        <Popover content={filterContent} placement="bottom" arrow={false}>
          <Button icon={<FilterIcon />} />
        </Popover>
        <Button
          icon={<PlusOutlined />}
          type="primary"
          onClick={() => createChunk()}
        />
      </Space>
    </Flex>
  );
};

export default ChunkToolBar;
