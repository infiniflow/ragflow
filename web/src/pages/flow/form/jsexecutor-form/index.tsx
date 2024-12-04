import { useTranslate } from '@/hooks/common-hooks';
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import MonacoEditor from '@monaco-editor/react';
import { Button, Form, Input, Space } from 'antd';
import { useEffect, useMemo } from 'react';
import { IOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import styles from './index.less';

// 出现了
const JSExecutorForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const { edges, nodes, setEdgesByNodeId } = useGraphStore();

  // 获取连接的上游节点信息
  const connectedNodes = useMemo(() => {
    if (!node?.id) return {};

    return edges.reduce((acc: Record<string, string>, edge) => {
      if (edge.target === node.id) {
        const sourceNode = nodes.find((n) => n.id === edge.source);
        if (sourceNode && edge.targetHandle) {
          const handleIndex = edge.targetHandle.split('-')[1];
          acc[handleIndex] = sourceNode.data.name;
        }
      }
      return acc;
    }, {});
  }, [edges, nodes, node?.id]);

  // 监听输入变量变化,同步更新连接点
  useEffect(() => {
    if (!form || !node?.id) return;

    const inputNames = form.getFieldValue('input_names') || [];
    const newEdges = inputNames.map((_: any, index: number) => ({
      id: `jsexecutor-${node.id}-${index}`,
      source: '',
      target: node.id,
      targetHandle: `input-${index}`,
    }));
    setEdgesByNodeId(node.id, newEdges);
  }, [form?.getFieldValue('input_names'), node?.id]);

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      {/* 输入变量列表 */}
      <Form.List name="input_names">
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name, ...restField }, index) => (
              <Space
                key={key}
                style={{ display: 'flex', marginBottom: 8 }}
                align="baseline"
              >
                <Form.Item
                  {...restField}
                  name={[name]}
                  rules={[{ required: true, message: t('inputNameRequired') }]}
                >
                  <Input placeholder={t('inputNamePlaceholder')} />
                </Form.Item>
                {connectedNodes[index] && (
                  <div className={styles.connectedNode}>
                    {connectedNodes[index]}
                  </div>
                )}
                {fields.length > 1 && (
                  <MinusCircleOutlined onClick={() => remove(name)} />
                )}
              </Space>
            ))}
            <Form.Item>
              <Button
                type="dashed"
                onClick={() => add()}
                block
                icon={<PlusOutlined />}
              >
                {t('addInputVariable')}
              </Button>
            </Form.Item>
          </>
        )}
      </Form.List>

      {/* JavaScript编辑器 */}
      <Form.Item label={t('script')} name="script">
        <MonacoEditor
          height="300px"
          defaultLanguage="javascript"
          theme="vs-dark"
          options={{
            minimap: { enabled: false },
            scrollBeyondLastLine: false,
            automaticLayout: true,
          }}
        />
      </Form.Item>

      {/* 使用说明 */}
      <div className={styles.helpText}>
        <h4>{t('scriptHelp')}</h4>
        <pre>{t('scriptHelpContent')}</pre>
      </div>
    </Form>
  );
};

export default JSExecutorForm;
