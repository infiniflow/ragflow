import SimilaritySlider from '@/components/similarity-slider';
import { DeleteOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import {
  Button,
  Col,
  Divider,
  Form,
  Input,
  Row,
  Slider,
  Switch,
  Table,
  TableProps,
  Tooltip,
} from 'antd';
import classNames from 'classnames';
import {
  ForwardedRef,
  forwardRef,
  useEffect,
  useImperativeHandle,
  useState,
} from 'react';
import { v4 as uuid } from 'uuid';
import {
  VariableTableDataType as DataType,
  IPromptConfigParameters,
  ISegmentedContentProps,
} from '../interface';
import { EditableCell, EditableRow } from './editable-cell';

import { useSelectPromptConfigParameters } from '../hooks';
import styles from './index.less';

type FieldType = {
  similarity_threshold?: number;
  vector_similarity_weight?: number;
  top_n?: number;
};

const PromptEngine = (
  { show }: ISegmentedContentProps,
  ref: ForwardedRef<Array<IPromptConfigParameters>>,
) => {
  const [dataSource, setDataSource] = useState<DataType[]>([]);
  const parameters = useSelectPromptConfigParameters();

  const components = {
    body: {
      row: EditableRow,
      cell: EditableCell,
    },
  };

  const handleRemove = (key: string) => () => {
    const newData = dataSource.filter((item) => item.key !== key);
    setDataSource(newData);
  };

  const handleSave = (row: DataType) => {
    const newData = [...dataSource];
    const index = newData.findIndex((item) => row.key === item.key);
    const item = newData[index];
    newData.splice(index, 1, {
      ...item,
      ...row,
    });
    setDataSource(newData);
  };

  const handleAdd = () => {
    setDataSource((state) => [
      ...state,
      {
        key: uuid(),
        variable: '',
        optional: true,
      },
    ]);
  };

  const handleOptionalChange = (row: DataType) => (checked: boolean) => {
    const newData = [...dataSource];
    const index = newData.findIndex((item) => row.key === item.key);
    const item = newData[index];
    newData.splice(index, 1, {
      ...item,
      optional: checked,
    });
    setDataSource(newData);
  };

  useImperativeHandle(
    ref,
    () => {
      return dataSource
        .filter((x) => x.variable.trim() !== '')
        .map((x) => ({ key: x.variable, optional: x.optional }));
    },
    [dataSource],
  );

  const columns: TableProps<DataType>['columns'] = [
    {
      title: 'key',
      dataIndex: 'variable',
      key: 'variable',
      onCell: (record: DataType) => ({
        record,
        editable: true,
        dataIndex: 'variable',
        title: 'key',
        handleSave,
      }),
    },
    {
      title: 'optional',
      dataIndex: 'optional',
      key: 'optional',
      width: 40,
      align: 'center',
      render(text, record) {
        return (
          <Switch
            size="small"
            checked={text}
            onChange={handleOptionalChange(record)}
          />
        );
      },
    },
    {
      title: 'operation',
      dataIndex: 'operation',
      width: 30,
      key: 'operation',
      align: 'center',
      render(_, record) {
        return <DeleteOutlined onClick={handleRemove(record.key)} />;
      },
    },
  ];

  useEffect(() => {
    setDataSource(parameters);
  }, [parameters]);

  return (
    <section
      className={classNames({
        [styles.segmentedHidden]: !show,
      })}
    >
      <Form.Item
        label="System"
        rules={[{ required: true, message: 'Please input!' }]}
        tooltip="Instructions you need LLM to follow when LLM answers questions, like charactor design, answer length and answer language etc."
        name={['prompt_config', 'system']}
        initialValue={`你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。
        以下是知识库：
        {knowledge}
        以上是知识库。`}
      >
        <Input.TextArea autoSize={{ maxRows: 8, minRows: 5 }} />
      </Form.Item>
      <Divider></Divider>
      <SimilaritySlider isTooltipShown></SimilaritySlider>
      <Form.Item<FieldType>
        label="Top N"
        name={'top_n'}
        initialValue={8}
        tooltip={`Not all the chunks whose similarity score is above the 'simialrity threashold' will be feed to LLMs. LLM can only see these 'Top N' chunks.`}
      >
        <Slider max={30} />
      </Form.Item>
      <section className={classNames(styles.variableContainer)}>
        <Row align={'middle'} justify="end">
          <Col span={7} className={styles.variableAlign}>
            <label className={styles.variableLabel}>
              Variables
              <Tooltip title="If you use dialog APIs, the varialbes might help you chat with your clients with different strategies. 
              The variables are used to fill-in the 'System' part in prompt in order to give LLM a hint.
              The 'knowledge' is a very special variable which will be filled-in with the retrieved chunks.
              All the variables in 'System' should be curly bracketed.">
                <QuestionCircleOutlined className={styles.variableIcon} />
              </Tooltip>
            </label>
          </Col>
          <Col span={17} className={styles.variableAlign}>
            <Button size="small" onClick={handleAdd}>
              Add
            </Button>
          </Col>
        </Row>
        {dataSource.length > 0 && (
          <Row>
            <Col span={7}> </Col>
            <Col span={17}>
              <Table
                dataSource={dataSource}
                columns={columns}
                rowKey={'key'}
                className={styles.variableTable}
                components={components}
                rowClassName={() => styles.editableRow}
              />
            </Col>
          </Row>
        )}
      </section>
    </section>
  );
};

export default forwardRef(PromptEngine);
