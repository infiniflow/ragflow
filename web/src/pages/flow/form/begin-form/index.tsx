import { PlusOutlined } from '@ant-design/icons';
import { Button, Form, Input } from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginQuery, IOperatorForm } from '../../interface';
import { useEditQueryRecord } from './hooks';
import { ModalForm } from './paramater-modal';
import QueryTable from './query-table';

import styles from './index.less';

type FieldType = {
  prologue?: string;
};

const BeginForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslation();
  const {
    ok,
    currentRecord,
    visible,
    hideModal,
    showModal,
    otherThanCurrentQuery,
  } = useEditQueryRecord({
    form,
    onValuesChange,
  });

  const handleDeleteRecord = useCallback(
    (idx: number) => {
      const query = form?.getFieldValue('query') || [];
      const nextQuery = query.filter(
        (item: BeginQuery, index: number) => index !== idx,
      );
      onValuesChange?.(
        { query: nextQuery },
        { query: nextQuery, prologue: form?.getFieldValue('prologue') },
      );
    },
    [form, onValuesChange],
  );

  return (
    <Form.Provider
      onFormFinish={(name, { values }) => {
        if (name === 'queryForm') {
          ok(values as BeginQuery);
        }
      }}
    >
      <Form
        name="basicForm"
        onValuesChange={onValuesChange}
        autoComplete="off"
        form={form}
        layout="vertical"
      >
        <Form.Item<FieldType>
          name={'prologue'}
          label={t('chat.setAnOpener')}
          tooltip={t('chat.setAnOpenerTip')}
          initialValue={t('chat.setAnOpenerInitial')}
        >
          <Input.TextArea autoSize={{ minRows: 5 }} />
        </Form.Item>
        {/* Create a hidden field to make Form instance record this */}
        <Form.Item name="query" noStyle />

        <Form.Item
          shouldUpdate={(prevValues, curValues) =>
            prevValues.query !== curValues.query
          }
        >
          {({ getFieldValue }) => {
            const query: BeginQuery[] = getFieldValue('query') || [];
            return (
              <QueryTable
                data={query}
                showModal={showModal}
                deleteRecord={handleDeleteRecord}
              ></QueryTable>
            );
          }}
        </Form.Item>

        <Button
          htmlType="button"
          style={{ margin: '0 8px' }}
          onClick={() => showModal()}
          icon={<PlusOutlined />}
          block
          className={styles.addButton}
        >
          {t('flow.addItem')}
        </Button>
        {visible && (
          <ModalForm
            visible={visible}
            hideModal={hideModal}
            initialValue={currentRecord}
            onOk={ok}
            otherThanCurrentQuery={otherThanCurrentQuery}
          />
        )}
      </Form>
    </Form.Provider>
  );
};

export default BeginForm;
