import TopNItem from '@/components/top-n-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { DeepLSourceLangOptions, DeepLTargetLangOptions } from '../../constant';
import { useBuildSortOptions } from '../../form-hooks';
import { IOperatorForm } from '../../interface';

const DeepLForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const options = useBuildSortOptions();

  return (
    <Form
      name="basic"
      labelCol={{ span: 8 }}
      wrapperCol={{ span: 16 }}
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
    >
      <TopNItem initialValue={5}></TopNItem>
      <Form.Item label={t('authKey')} name={'auth_key'}>
        <Select options={options}></Select>
      </Form.Item>
      <Form.Item label={t('sourceLang')} name={'source_lang'}>
        <Select options={DeepLSourceLangOptions}></Select>
      </Form.Item>
      <Form.Item label={t('targetLang')} name={'target_lang'}>
        <Select options={DeepLTargetLangOptions}></Select>
      </Form.Item>
    </Form>
  );
};

export default DeepLForm;
