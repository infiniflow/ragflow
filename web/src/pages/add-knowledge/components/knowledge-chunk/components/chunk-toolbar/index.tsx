import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
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

const ChunkToolBar = () => {
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
      <Space>
        <ArrowLeftOutlined />
        <FilePdfOutlined />
        xxx.pdf
      </Space>
      <Space>
        {/* <Select
          defaultValue="lucy"
          style={{ width: 100 }}
          popupMatchSelectWidth={false}
          optionRender={() => null}
          dropdownRender={(menu) => (
            <div style={{ width: 300 }}>
              {menu}
              <Menu
                // onClick={onClick}
                style={{ width: 256 }}
                // defaultSelectedKeys={['1']}
                // defaultOpenKeys={['sub1']}
                // mode="inline"
                items={actionItems}
              />
            </div>
          )}
        ></Select> */}
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
