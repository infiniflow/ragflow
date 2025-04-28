import React, { useState } from 'react';
import { Button, Table, Space, Modal, Form, Input, Tooltip } from 'antd';
import { useTranslation } from 'react-i18next';
import { PlusOutlined, DeleteOutlined, EditOutlined, UserAddOutlined } from '@ant-design/icons';
import { useListDepartment, useDeleteDepartment, useAddDepartment, useUpdateDepartment } from '@/hooks/user-setting-hooks';
import { formatDate } from '@/utils/date';
import type { TableProps } from 'antd';

interface IDepartment {
  id: string;
  name: string;
  description?: string;
  member_count: number;
  create_date: string;
}

const DepartmentTable: React.FC = () => {
  const { t } = useTranslation();
  const { data, loading, refetch } = useListDepartment();
  const { deleteDepartment } = useDeleteDepartment();
  const { addDepartment } = useAddDepartment();
  const { updateDepartment } = useUpdateDepartment();
  const [isAddModalVisible, setIsAddModalVisible] = useState(false);
  const [isEditModalVisible, setIsEditModalVisible] = useState(false);
  const [currentDepartment, setCurrentDepartment] = useState<IDepartment | null>(null);
  const [form] = Form.useForm();

  const handleDelete = (id: string) => {
    Modal.confirm({
      title: t('common.confirm'),
      content: t('setting.confirmDelete'),
      onOk: async () => {
        await deleteDepartment(id);
        refetch();
      },
    });
  };

  const handleAdd = () => {
    form.resetFields();
    setIsAddModalVisible(true);
  };

  const handleEdit = (record: IDepartment) => {
    setCurrentDepartment(record);
    form.setFieldsValue({
      name: record.name,
      description: record.description,
    });
    setIsEditModalVisible(true);
  };

  const handleAddSubmit = async () => {
    try {
      const values = await form.validateFields();
      await addDepartment(values);
      setIsAddModalVisible(false);
      refetch();
    } catch (error) {
      console.error('验证失败:', error);
    }
  };

  const handleEditSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (currentDepartment) {
        await updateDepartment({ id: currentDepartment.id, ...values });
        setIsEditModalVisible(false);
        refetch();
      }
    } catch (error) {
      console.error('验证失败:', error);
    }
  };

  const handleAddMember = (departmentId: string) => {
    // 此功能将在后续实现
    Modal.info({
      title: '功能提示',
      content: '添加成员功能即将上线',
    });
  };

  const columns: TableProps<IDepartment>['columns'] = [
    {
      title: t('common.name'),
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
    },
    {
      title: '成员数量',
      dataIndex: 'member_count',
      key: 'member_count',
    },
    {
      title: t('setting.createDate'),
      dataIndex: 'create_date',
      key: 'create_date',
      render(value) {
        return formatDate(value);
      },
    },
    {
      title: t('common.action'),
      key: 'action',
      render: (_, record) => (
        <Space>
          <Tooltip title="添加成员">
            <Button type="text" onClick={() => handleAddMember(record.id)}>
              <UserAddOutlined />
            </Button>
          </Tooltip>
          <Tooltip title={t('common.edit')}>
            <Button type="text" onClick={() => handleEdit(record)}>
              <EditOutlined />
            </Button>
          </Tooltip>
          <Tooltip title={t('common.delete')}>
            <Button type="text" onClick={() => handleDelete(record.id)}>
              <DeleteOutlined />
            </Button>
          </Tooltip>
        </Space>
      ),
    },
  ];

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
        <Button type="primary" onClick={handleAdd} icon={<PlusOutlined />}>
          添加部门
        </Button>
      </div>
      <Table
        dataSource={data}
        columns={columns}
        rowKey="id"
        loading={loading}
        pagination={false}
        style={{ width: '100%' }}
      />

      {/* 添加部门的弹窗 */}
      <Modal
        title="添加部门"
        open={isAddModalVisible}
        onOk={handleAddSubmit}
        onCancel={() => setIsAddModalVisible(false)}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label={t('common.name')}
            rules={[{ required: true, message: '请输入部门名称' }]}
          >
            <Input placeholder="请输入部门名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea placeholder="请输入部门描述" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 编辑部门的弹窗 */}
      <Modal
        title="编辑部门"
        open={isEditModalVisible}
        onOk={handleEditSubmit}
        onCancel={() => setIsEditModalVisible(false)}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label={t('common.name')}
            rules={[{ required: true, message: '请输入部门名称' }]}
          >
            <Input placeholder="请输入部门名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea placeholder="请输入部门描述" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default DepartmentTable;
