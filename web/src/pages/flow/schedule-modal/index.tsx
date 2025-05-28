import { useTranslate } from '@/hooks/common-hooks';
import {
  useCreateSchedule,
  useDeleteSchedule,
  useFetchFrequencyOptions,
  useFetchSchedules,
  useToggleSchedule,
  useUpdateSchedule,
} from '@/hooks/schedule-hooks';
import {
  ICreateScheduleRequest,
  ISchedule,
} from '@/interfaces/database/schedule';
import { DeleteOutlined, EditOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Select,
  Spin,
  Switch,
  Table,
  TimePicker,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import timezone from 'dayjs/plugin/timezone';
import utc from 'dayjs/plugin/utc';
import { useCallback, useState } from 'react';

// Configure dayjs plugins
dayjs.extend(utc);
dayjs.extend(timezone);

const { Text } = Typography;
const { TextArea } = Input;

interface ScheduleModalProps {
  visible: boolean;
  hideModal: () => void;
  canvasId: string;
  canvasTitle: string;
}

export function ScheduleModal({
  visible,
  hideModal,
  canvasId,
  canvasTitle,
}: ScheduleModalProps) {
  const { t } = useTranslate('flow');
  const [form] = Form.useForm();
  const [editingSchedule, setEditingSchedule] = useState<ISchedule | null>(
    null,
  );
  const [isCreating, setIsCreating] = useState(false);

  const { data: frequencyOptions, loading: loadingOptions } =
    useFetchFrequencyOptions();
  const {
    schedules,
    total,
    loading: loadingSchedules,
    refetch,
  } = useFetchSchedules();
  const { createSchedule, loading: creating } = useCreateSchedule();
  const { updateSchedule, loading: updating } = useUpdateSchedule();
  const { toggleSchedule, loading: toggling } = useToggleSchedule();
  const { deleteSchedule, loading: deleting } = useDeleteSchedule();

  const frequencyType = Form.useWatch('frequency_type', form);

  const handleCreateNew = useCallback(() => {
    setIsCreating(true);
    setEditingSchedule(null);
    form.resetFields();

    // Set default values with proper dayjs objects
    const defaultTime = dayjs().add(1, 'hour');
    form.setFieldsValue({
      frequency_type: 'once',
      execute_time: defaultTime,
      execute_date: defaultTime,
    });
  }, [form]);

  const handleEdit = useCallback(
    (schedule: ISchedule) => {
      setIsCreating(true);
      setEditingSchedule(schedule);

      const formData: any = {
        name: schedule.name,
        description: schedule.description,
        frequency_type: schedule.frequency_type,
        days_of_week: schedule.days_of_week,
        day_of_month: schedule.day_of_month,
      };

      // Handle time conversion
      if (schedule.execute_time) {
        try {
          // Parse time string to dayjs object
          const timeStr = schedule.execute_time;
          const timeParts = timeStr.split(':');
          const hours = parseInt(timeParts[0], 10);
          const minutes = parseInt(timeParts[1], 10);
          const seconds = parseInt(timeParts[2] || '0', 10);

          formData.execute_time = dayjs()
            .hour(hours)
            .minute(minutes)
            .second(seconds);
        } catch (error) {
          console.warn('Failed to parse execute_time:', schedule.execute_time);
          formData.execute_time = dayjs();
        }
      }

      // Handle date conversion
      if (schedule.execute_date) {
        try {
          formData.execute_date = dayjs(schedule.execute_date);
        } catch (error) {
          console.warn('Failed to parse execute_date:', schedule.execute_date);
          formData.execute_date = dayjs();
        }
      }

      form.setFieldsValue(formData);
    },
    [form],
  );

  const handleCancel = useCallback(() => {
    setIsCreating(false);
    setEditingSchedule(null);
    form.resetFields();
  }, [form]);

  const handleSave = useCallback(async () => {
    try {
      const values = await form.validateFields();

      const payload: ICreateScheduleRequest = {
        canvas_id: canvasId,
        name: values.name,
        description: values.description,
        frequency_type: values.frequency_type,
      };

      // Handle time conversion
      if (values.execute_time && dayjs.isDayjs(values.execute_time)) {
        payload.execute_time = values.execute_time.format('HH:mm:ss');
      }

      // Handle date conversion
      if (values.execute_date && dayjs.isDayjs(values.execute_date)) {
        payload.execute_date = values.execute_date.toISOString();
      }

      if (values.days_of_week) {
        payload.days_of_week = values.days_of_week;
      }

      if (values.day_of_month) {
        payload.day_of_month = values.day_of_month;
      }

      if (editingSchedule) {
        await updateSchedule({ id: editingSchedule.id, ...payload });
      } else {
        await createSchedule(payload);
      }

      handleCancel();
      refetch();
    } catch (error) {
      console.error('Form validation failed:', error);
    }
  }, [
    form,
    canvasId,
    editingSchedule,
    createSchedule,
    updateSchedule,
    handleCancel,
    refetch,
  ]);

  const handleToggle = useCallback(
    async (scheduleId: string) => {
      await toggleSchedule(scheduleId);
      refetch();
    },
    [toggleSchedule, refetch],
  );

  const handleDelete = useCallback(
    async (scheduleId: string) => {
      await deleteSchedule(scheduleId);
      refetch();
    },
    [deleteSchedule, refetch],
  );

  const getRequiredFields = useCallback(() => {
    if (!frequencyOptions?.frequency_types || !frequencyType) return [];

    const option = frequencyOptions.frequency_types.find(
      (type) => type.value === frequencyType,
    );
    return option?.required_fields || [];
  }, [frequencyOptions, frequencyType]);

  // Utility function to safely format date/time
  const formatDateTime = useCallback((timestamp: number | string) => {
    try {
      const date =
        typeof timestamp === 'number'
          ? dayjs(timestamp * 1000)
          : dayjs(timestamp);

      return date.isValid() ? date.format('YYYY-MM-DD HH:mm:ss') : '-';
    } catch (error) {
      console.warn('Failed to format date:', timestamp);
      return '-';
    }
  }, []);

  const columns = [
    {
      title: t('schedule.name'),
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: ISchedule) => (
        <div>
          <div className="font-medium">{text}</div>
          {record.description && (
            <Text type="secondary" className="text-xs">
              {record.description}
            </Text>
          )}
        </div>
      ),
    },
    {
      title: t('schedule.frequency'),
      dataIndex: 'frequency_type',
      key: 'frequency_type',
      render: (type: string, record: ISchedule) => {
        if (!frequencyOptions?.frequency_types) {
          return type;
        }

        const option = frequencyOptions.frequency_types.find(
          (t) => t.value === type,
        );
        let details = option?.label || type;

        if (
          type === 'weekly' &&
          record.days_of_week?.length &&
          frequencyOptions?.days_of_week
        ) {
          const dayNames = record.days_of_week
            .map(
              (day) =>
                frequencyOptions.days_of_week.find((d) => d.value === day)
                  ?.label,
            )
            .filter(Boolean)
            .join(', ');
          details += ` (${dayNames})`;
        } else if (type === 'monthly' && record.day_of_month) {
          details += ` (${t('schedule.day')} ${record.day_of_month})`;
        }

        if (record.execute_time) {
          details += ` ${t('schedule.at')} ${record.execute_time}`;
        }

        return details;
      },
    },
    {
      title: t('schedule.nextRun'),
      dataIndex: 'next_run_time',
      key: 'next_run_time',
      render: (time: number) => formatDateTime(time),
    },
    {
      title: t('schedule.runCount'),
      dataIndex: 'run_count',
      key: 'run_count',
    },
    {
      title: t('schedule.status'),
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean, record: ISchedule) => (
        <Switch
          checked={enabled}
          loading={toggling}
          onChange={() => handleToggle(record.id)}
          checkedChildren={t('schedule.enabled')}
          unCheckedChildren={t('schedule.disabled')}
        />
      ),
    },
    {
      title: t('common.action'),
      key: 'action',
      width: 80,
      render: (_: any, record: ISchedule) => (
        <div className="flex items-center gap-2">
          <Tooltip title={t('common.edit')}>
            <Button
              type="text"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
              className="flex items-center justify-center"
            />
          </Tooltip>
          <Popconfirm
            title={t('schedule.deleteConfirm')}
            onConfirm={() => handleDelete(record.id)}
            okText={t('common.yes')}
            cancelText={t('common.no')}
          >
            <Tooltip title={t('common.delete')}>
              <Button
                type="text"
                size="small"
                icon={<DeleteOutlined />}
                loading={deleting}
                danger
                className="flex items-center justify-center"
              />
            </Tooltip>
          </Popconfirm>
        </div>
      ),
    },
  ];

  const requiredFields = getRequiredFields();

  // Show loading or error states
  if (loadingOptions) {
    return (
      <Modal
        title={t('schedule.title')}
        open={visible}
        onCancel={hideModal}
        width={1200}
        footer={null}
        destroyOnClose
      >
        <div className="flex justify-center items-center h-32">
          <Spin size="large" />
        </div>
      </Modal>
    );
  }

  return (
    <Modal
      title={t('schedule.title')}
      open={visible}
      onCancel={hideModal}
      width={1200}
      footer={null}
      destroyOnClose
    >
      <div className="space-y-4">
        <Card
          title={
            <div className="flex justify-between items-center">
              <span>{t('schedule.for', { name: canvasTitle })}</span>
              <Button
                type="primary"
                onClick={handleCreateNew}
                disabled={!frequencyOptions}
              >
                {t('schedule.create')}
              </Button>
            </div>
          }
        >
          <Table
            columns={columns}
            dataSource={schedules}
            loading={loadingSchedules}
            rowKey="id"
            pagination={{
              total,
              pageSize: 20,
              showSizeChanger: true,
              showQuickJumper: true,
            }}
            scroll={{ x: 800 }}
          />
        </Card>

        {isCreating && (
          <Card
            title={editingSchedule ? t('schedule.edit') : t('schedule.create')}
            extra={<Button onClick={handleCancel}>{t('common.cancel')}</Button>}
          >
            <Form form={form} layout="vertical">
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    label={t('schedule.name')}
                    name="name"
                    rules={[
                      { required: true, message: t('schedule.nameRequired') },
                    ]}
                  >
                    <Input placeholder={t('schedule.namePlaceholder')} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item
                    label={t('schedule.frequency')}
                    name="frequency_type"
                    rules={[
                      {
                        required: true,
                        message: t('schedule.frequencyRequired'),
                      },
                    ]}
                  >
                    <Select
                      placeholder={t('schedule.frequencyPlaceholder')}
                      loading={loadingOptions}
                      optionLabelProp="label"
                      disabled={!frequencyOptions?.frequency_types}
                    >
                      {frequencyOptions?.frequency_types?.map((option) => (
                        <Select.Option
                          key={option.value}
                          value={option.value}
                          label={option.label}
                        >
                          <div className="py-1">
                            <div className="font-medium text-sm">
                              {option.label}
                            </div>
                            <div className="text-xs text-gray-500 mt-1 leading-tight">
                              {option.description}
                            </div>
                          </div>
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item label={t('schedule.description')} name="description">
                <TextArea
                  rows={2}
                  placeholder={t('schedule.descriptionPlaceholder')}
                />
              </Form.Item>

              {requiredFields.includes('execute_time') && (
                <Row gutter={16}>
                  <Col span={requiredFields.includes('execute_date') ? 12 : 24}>
                    <Form.Item
                      label={t('schedule.executeTime')}
                      name="execute_time"
                      rules={[
                        {
                          required: true,
                          message: t('schedule.executeTimeRequired'),
                        },
                      ]}
                    >
                      <TimePicker
                        format="HH:mm:ss"
                        className="w-full"
                        placeholder={t('schedule.executeTimePlaceholder')}
                        showNow={false}
                      />
                    </Form.Item>
                  </Col>
                  {requiredFields.includes('execute_date') && (
                    <Col span={12}>
                      <Form.Item
                        label={t('schedule.executeDate')}
                        name="execute_date"
                        rules={[
                          {
                            required: true,
                            message: t('schedule.executeDateRequired'),
                          },
                        ]}
                      >
                        <DatePicker
                          className="w-full"
                          format="YYYY-MM-DD"
                          placeholder={t('schedule.executeDatePlaceholder')}
                          disabledDate={(current) =>
                            current && current < dayjs().startOf('day')
                          }
                          showToday={true}
                        />
                      </Form.Item>
                    </Col>
                  )}
                </Row>
              )}

              {requiredFields.includes('days_of_week') && (
                <Form.Item
                  label={t('schedule.daysOfWeek')}
                  name="days_of_week"
                  rules={[
                    {
                      required: true,
                      message: t('schedule.daysOfWeekRequired'),
                    },
                  ]}
                >
                  <Select
                    mode="multiple"
                    placeholder={t('schedule.daysOfWeekPlaceholder')}
                    maxTagCount="responsive"
                    disabled={!frequencyOptions?.days_of_week}
                  >
                    {frequencyOptions?.days_of_week?.map((day) => (
                      <Select.Option key={day.value} value={day.value}>
                        {day.label}
                      </Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              )}

              {requiredFields.includes('day_of_month') && (
                <Form.Item
                  label={t('schedule.dayOfMonth')}
                  name="day_of_month"
                  rules={[
                    {
                      required: true,
                      message: t('schedule.dayOfMonthRequired'),
                    },
                  ]}
                >
                  <Select
                    placeholder={t('schedule.dayOfMonthPlaceholder')}
                    showSearch
                    filterOption={(input, option) =>
                      option?.children
                        ?.toString()
                        .toLowerCase()
                        .includes(input.toLowerCase()) ?? false
                    }
                  >
                    {Array.from({ length: 31 }, (_, i) => i + 1).map((day) => (
                      <Select.Option key={day} value={day}>
                        {day}
                      </Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              )}

              <div className="flex justify-end space-x-2 mt-6 pt-4 border-t border-gray-200">
                <Button onClick={handleCancel}>{t('common.cancel')}</Button>
                <Button
                  type="primary"
                  onClick={handleSave}
                  loading={creating || updating}
                  disabled={!frequencyOptions}
                >
                  {editingSchedule ? t('common.update') : t('common.create')}
                </Button>
              </div>
            </Form>
          </Card>
        )}
      </div>
    </Modal>
  );
}

export function useScheduleModal() {
  const [visible, setVisible] = useState(false);

  const showModal = useCallback(() => setVisible(true), []);
  const hideModal = useCallback(() => setVisible(false), []);

  return { visible, showModal, hideModal };
}
