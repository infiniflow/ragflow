import { useTranslate } from '@/hooks/common-hooks';
import {
  useCreateSchedule,
  useDeleteSchedule,
  useFetchFrequencyOptions,
  useFetchScheduleHistory,
  useFetchSchedules,
  useFetchScheduleStats,
  useToggleSchedule,
  useUpdateSchedule,
} from '@/hooks/schedule-hooks';
import {
  ICreateScheduleRequest,
  ISchedule,
  IScheduleRun,
  IScheduleStats,
} from '@/interfaces/database/schedule';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  HistoryOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  DatePicker,
  Drawer,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Select,
  Spin,
  Statistic,
  Switch,
  Table,
  Tag,
  TimePicker,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import timezone from 'dayjs/plugin/timezone';
import utc from 'dayjs/plugin/utc';
import React, { useCallback, useState } from 'react';

// Configure dayjs plugins
dayjs.extend(utc);
dayjs.extend(timezone);

const { Text } = Typography;
const { TextArea } = Input;

interface ScheduleFormModalProps {
  visible: boolean;
  onCancel: () => void;
  onSave: () => void;
  editingSchedule: ISchedule | null;
  canvasId: string;
  loading: boolean;
}

function ScheduleFormModal({
  visible,
  onCancel,
  onSave,
  editingSchedule,
  canvasId,
  loading,
}: ScheduleFormModalProps) {
  const { t } = useTranslate('flow');
  const [form] = Form.useForm();

  const { data: frequencyOptions, loading: loadingOptions } =
    useFetchFrequencyOptions();
  const { createSchedule, loading: creating } = useCreateSchedule();
  const { updateSchedule, loading: updating } = useUpdateSchedule();

  const frequencyType = Form.useWatch('frequency_type', form);

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

      form.resetFields();
      onSave();
    } catch (error) {
      console.error('Form validation failed:', error);
    }
  }, [form, canvasId, editingSchedule, createSchedule, updateSchedule, onSave]);

  const getRequiredFields = useCallback(() => {
    if (!frequencyOptions?.frequency_types || !frequencyType) return [];

    const option = frequencyOptions.frequency_types.find(
      (type) => type.value === frequencyType,
    );
    return option?.required_fields || [];
  }, [frequencyOptions, frequencyType]);

  // Set form values when editing schedule changes
  React.useEffect(() => {
    if (visible && editingSchedule) {
      const formData: any = {
        name: editingSchedule.name,
        description: editingSchedule.description,
        frequency_type: editingSchedule.frequency_type,
        days_of_week: editingSchedule.days_of_week,
        day_of_month: editingSchedule.day_of_month,
      };

      // Handle time conversion
      if (editingSchedule.execute_time) {
        try {
          const timeStr = editingSchedule.execute_time;
          const timeParts = timeStr.split(':');
          const hours = parseInt(timeParts[0], 10);
          const minutes = parseInt(timeParts[1], 10);
          const seconds = parseInt(timeParts[2] || '0', 10);

          formData.execute_time = dayjs()
            .hour(hours)
            .minute(minutes)
            .second(seconds);
        } catch (error) {
          console.warn(
            'Failed to parse execute_time:',
            editingSchedule.execute_time,
          );
          formData.execute_time = dayjs();
        }
      }

      // Handle date conversion
      if (editingSchedule.execute_date) {
        try {
          formData.execute_date = dayjs(editingSchedule.execute_date);
        } catch (error) {
          console.warn(
            'Failed to parse execute_date:',
            editingSchedule.execute_date,
          );
          formData.execute_date = dayjs();
        }
      }

      form.setFieldsValue(formData);
    } else if (visible && !editingSchedule) {
      // Set default values for new schedule
      form.resetFields();
      const defaultTime = dayjs().add(1, 'hour');
      form.setFieldsValue({
        frequency_type: 'once',
        execute_time: defaultTime,
        execute_date: defaultTime,
      });
    }
  }, [visible, editingSchedule, form]);

  const requiredFields = getRequiredFields();

  return (
    <Modal
      title={editingSchedule ? t('schedule.edit') : t('schedule.create')}
      open={visible}
      onCancel={onCancel}
      width={800}
      footer={[
        <Button key="cancel" onClick={onCancel}>
          {t('common.cancel')}
        </Button>,
        <Button
          key="save"
          type="primary"
          onClick={handleSave}
          loading={creating || updating || loading}
          disabled={!frequencyOptions}
        >
          {editingSchedule ? t('common.update') : t('common.create')}
        </Button>,
      ]}
      destroyOnClose
    >
      <Form form={form} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              label={t('schedule.name')}
              name="name"
              rules={[{ required: true, message: t('schedule.nameRequired') }]}
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
                      <div className="font-medium text-sm">{option.label}</div>
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
      </Form>
    </Modal>
  );
}

interface ScheduleRunDrawerProps {
  visible: boolean;
  onClose: () => void;
  schedule: ISchedule | null;
}

function ScheduleRunDrawer({
  visible,
  onClose,
  schedule,
}: ScheduleRunDrawerProps) {
  const { t } = useTranslate('flow');

  const {
    stats,
    loading: loadingStats,
    refetch: refetchStats,
  } = useFetchScheduleStats(schedule?.id || '');
  const {
    history,
    loading: loadingHistory,
    refetch: refetchHistory,
  } = useFetchScheduleHistory(schedule?.id || '');

  const formatDateTime = useCallback((dateTime: string) => {
    try {
      return dayjs(dateTime).format('YYYY-MM-DD HH:mm:ss');
    } catch (error) {
      return '-';
    }
  }, []);

  const calculateDuration = useCallback(
    (startTime: string, endTime: string | null) => {
      if (!endTime) return null;

      try {
        let start: dayjs.Dayjs;
        let end: dayjs.Dayjs;

        start = dayjs(startTime);

        end = dayjs(endTime);

        return end.diff(start, 'seconds');
      } catch (error) {
        return null;
      }
    },
    [],
  );

  const formatDuration = useCallback((duration: number | null) => {
    if (!duration || duration <= 0) return '-';

    const minutes = Math.floor(duration / 60);
    const seconds = Math.floor(duration % 60);

    if (minutes > 0) {
      return `${minutes}m ${seconds}s`;
    }
    return `${seconds}s`;
  }, []);

  const getStatusTag = useCallback(
    (run: IScheduleRun) => {
      if (run.finished_at === null || run.finished_at === undefined) {
        return (
          <Tag icon={<ClockCircleOutlined />} color="processing">
            {t('schedule.running')}
          </Tag>
        );
      }

      if (run.success) {
        return (
          <Tag icon={<CheckCircleOutlined />} color="success">
            {t('schedule.success')}
          </Tag>
        );
      }

      return (
        <Tag icon={<CloseCircleOutlined />} color="error">
          {t('schedule.failed')}
        </Tag>
      );
    },
    [t],
  );

  const handleRefresh = useCallback(() => {
    refetchStats();
    refetchHistory();
  }, [refetchStats, refetchHistory]);

  const historyColumns = [
    {
      title: t('schedule.startTime'),
      dataIndex: 'started_at',
      key: 'started_at',
      render: (time: string | number) => formatDateTime(time),
      sorter: (a: IScheduleRun, b: IScheduleRun) => {
        const aTime =
          typeof a.started_at === 'number'
            ? a.started_at
            : dayjs(a.started_at).valueOf();
        const bTime =
          typeof b.started_at === 'number'
            ? b.started_at
            : dayjs(b.started_at).valueOf();
        return bTime - aTime;
      },
      defaultSortOrder: 'descend' as const,
    },
    {
      title: t('schedule.endTime'),
      dataIndex: 'finished_at',
      key: 'finished_at',
      render: (time: string | number | null) =>
        time ? formatDateTime(time) : t('schedule.running'),
    },
    {
      title: t('schedule.duration'),
      key: 'duration',
      render: (record: IScheduleRun) => {
        const duration = calculateDuration(
          record.started_at,
          record.finished_at,
        );
        return formatDuration(duration);
      },
    },
    {
      title: t('schedule.status'),
      key: 'status',
      render: (record: IScheduleRun) => getStatusTag(record),
    },
    {
      title: t('schedule.errorMessage'),
      dataIndex: 'error_message',
      key: 'error_message',
      render: (message: string) =>
        message ? (
          <Tooltip title={message}>
            <Text type="danger" className="text-xs cursor-pointer">
              {message.slice(0, 30)}
              {message.length > 30 ? '...' : ''}
            </Text>
          </Tooltip>
        ) : (
          '-'
        ),
    },
  ];

  return (
    <Drawer
      title={
        <div className="flex items-center justify-between">
          <span>
            {t('schedule.runInfo')} - {schedule?.name}
          </span>
          <Button
            icon={<ReloadOutlined />}
            onClick={handleRefresh}
            loading={loadingStats || loadingHistory}
            size="small"
          >
            {t('common.refresh')}
          </Button>
        </div>
      }
      open={visible}
      onClose={onClose}
      width={1000}
      destroyOnClose
    >
      {schedule && (
        <div className="space-y-6">
          {/* Stats Section */}
          <Card title={t('schedule.statistics')} loading={loadingStats}>
            <Row gutter={16}>
              <Col span={6}>
                <Statistic
                  title={t('schedule.totalRuns')}
                  value={stats.total_runs || 0}
                  prefix={<HistoryOutlined />}
                />
              </Col>
              <Col span={6}>
                <Statistic
                  title={t('schedule.successfulRuns')}
                  value={stats.successful_runs || 0}
                  prefix={<CheckCircleOutlined />}
                  valueStyle={{ color: '#3f8600' }}
                />
              </Col>
              <Col span={6}>
                <Statistic
                  title={t('schedule.failedRuns')}
                  value={stats.failed_runs || 0}
                  prefix={<CloseCircleOutlined />}
                  valueStyle={{ color: '#cf1322' }}
                />
              </Col>
              <Col span={6}>
                <div className="text-center">
                  <div className="text-sm text-gray-500 mb-1">
                    {t('schedule.currentStatus')}
                  </div>
                  <Badge
                    status={
                      stats.is_currently_running ? 'processing' : 'default'
                    }
                    text={
                      stats.is_currently_running
                        ? t('schedule.running')
                        : t('schedule.idle')
                    }
                  />
                </div>
              </Col>
            </Row>

            {(stats as IScheduleStats).last_successful_run && (
              <div className="mt-4 pt-4 border-t">
                <Text strong>{t('schedule.lastSuccessfulRun')}: </Text>
                <Text>
                  {formatDateTime(stats.last_successful_run.started_at)}
                </Text>
              </div>
            )}
          </Card>

          {/* Current Status Alert */}
          {stats.is_currently_running && (
            <Alert
              message={t('schedule.currentlyRunning')}
              description={t('schedule.currentlyRunningDesc')}
              type="info"
              icon={<ClockCircleOutlined />}
              showIcon
            />
          )}

          {/* Execution History */}
          <Card title={t('schedule.executionHistory')} loading={loadingHistory}>
            <Table
              columns={historyColumns}
              dataSource={history}
              rowKey="id"
              pagination={{
                pageSize: 10,
                showSizeChanger: true,
                showQuickJumper: true,
                showTotal: (total, range) =>
                  `${range[0]}-${range[1]} ${t('common.of')} ${total} ${t('schedule.runs')}`,
              }}
              scroll={{ x: 800 }}
              size="small"
            />
          </Card>
        </div>
      )}
    </Drawer>
  );
}

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
  const [editingSchedule, setEditingSchedule] = useState<ISchedule | null>(
    null,
  );
  const [isFormVisible, setIsFormVisible] = useState(false);
  const [runDrawerVisible, setRunDrawerVisible] = useState(false);
  const [selectedSchedule, setSelectedSchedule] = useState<ISchedule | null>(
    null,
  );

  const { data: frequencyOptions, loading: loadingOptions } =
    useFetchFrequencyOptions();
  const {
    schedules,
    total,
    loading: loadingSchedules,
    refetch,
  } = useFetchSchedules();
  const { toggleSchedule, loading: toggling } = useToggleSchedule();
  const { deleteSchedule, loading: deleting } = useDeleteSchedule();

  const handleCreateNew = useCallback(() => {
    setEditingSchedule(null);
    setIsFormVisible(true);
  }, []);

  const handleEdit = useCallback((schedule: ISchedule) => {
    setEditingSchedule(schedule);
    setIsFormVisible(true);
  }, []);

  const handleFormCancel = useCallback(() => {
    setIsFormVisible(false);
    setEditingSchedule(null);
  }, []);

  const handleFormSave = useCallback(() => {
    setIsFormVisible(false);
    setEditingSchedule(null);
    refetch();
  }, [refetch]);

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

  const handleViewRuns = useCallback((schedule: ISchedule) => {
    setSelectedSchedule(schedule);
    setRunDrawerVisible(true);
  }, []);

  const handleCloseRunDrawer = useCallback(() => {
    setRunDrawerVisible(false);
    setSelectedSchedule(null);
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
      width: 120,
      render: (_: any, record: ISchedule) => (
        <div className="flex items-center gap-1">
          <Tooltip title={t('schedule.viewRuns')}>
            <Button
              type="text"
              size="small"
              icon={<HistoryOutlined />}
              onClick={() => handleViewRuns(record)}
              className="flex items-center justify-center"
            />
          </Tooltip>
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
    <>
      <Modal
        title={t('schedule.title')}
        open={visible}
        onCancel={hideModal}
        width={1200}
        footer={null}
        destroyOnClose
      >
        <Card
          title={
            <div className="flex justify-between items-center">
              <span>
                {t('schedule.for')} {canvasTitle}
              </span>
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
      </Modal>

      <ScheduleFormModal
        visible={isFormVisible}
        onCancel={handleFormCancel}
        onSave={handleFormSave}
        editingSchedule={editingSchedule}
        canvasId={canvasId}
        loading={loadingSchedules}
      />

      <ScheduleRunDrawer
        visible={runDrawerVisible}
        onClose={handleCloseRunDrawer}
        schedule={selectedSchedule}
      />
    </>
  );
}

export function useScheduleModal() {
  const [visible, setVisible] = useState(false);

  const showModal = useCallback(() => setVisible(true), []);
  const hideModal = useCallback(() => setVisible(false), []);

  return { visible, showModal, hideModal };
}
