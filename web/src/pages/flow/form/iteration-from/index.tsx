import { CommaIcon, SemicolonIcon } from '@/assets/icon/Icon';
import { Form, Select } from 'antd';
import {
  CornerDownLeft,
  IndentIncrease,
  Minus,
  Slash,
  Underline,
} from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const optionList = [
  {
    value: ',',
    icon: CommaIcon,
    text: 'comma',
  },
  {
    value: '\n',
    icon: CornerDownLeft,
    text: 'lineBreak',
  },
  {
    value: 'tab',
    icon: IndentIncrease,
    text: 'tab',
  },
  {
    value: '_',
    icon: Underline,
    text: 'underline',
  },
  {
    value: '/',
    icon: Slash,
    text: 'diagonal',
  },
  {
    value: '-',
    icon: Minus,
    text: 'minus',
  },
  {
    value: ';',
    icon: SemicolonIcon,
    text: 'semicolon',
  },
];

const IterationForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslation();

  const options = useMemo(() => {
    return optionList.map((x) => {
      let Icon = x.icon;

      return {
        value: x.value,
        label: (
          <div className="flex items-center gap-2">
            <Icon className={'size-4'}></Icon>
            {t(`flow.delimiterOptions.${x.text}`)}
          </div>
        ),
      };
    });
  }, [t]);

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <Form.Item
        name={['delimiter']}
        label={t('knowledgeDetails.delimiter')}
        initialValue={`\\n!?;。；！？`}
        rules={[{ required: true }]}
        tooltip={t('flow.delimiterTip')}
      >
        <Select options={options}></Select>
      </Form.Item>
    </Form>
  );
};

export default IterationForm;
