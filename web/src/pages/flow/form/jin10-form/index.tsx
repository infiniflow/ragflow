import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Select } from 'antd';
import { useMemo } from 'react';
import {
  Jin10CalendarDatashapeOptions,
  Jin10CalendarTypeOptions,
  Jin10FlashTypeOptions,
  Jin10SymbolsDatatypeOptions,
  Jin10SymbolsTypeOptions,
  Jin10TypeOptions,
} from '../../constant';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const Jin10Form = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  const jin10TypeOptions = useMemo(() => {
    return Jin10TypeOptions.map((x) => ({
      value: x,
      label: t(`jin10TypeOptions.${x}`),
    }));
  }, [t]);

  const jin10FlashTypeOptions = useMemo(() => {
    return Jin10FlashTypeOptions.map((x) => ({
      value: x,
      label: t(`jin10FlashTypeOptions.${x}`),
    }));
  }, [t]);

  const jin10CalendarTypeOptions = useMemo(() => {
    return Jin10CalendarTypeOptions.map((x) => ({
      value: x,
      label: t(`jin10CalendarTypeOptions.${x}`),
    }));
  }, [t]);

  const jin10CalendarDatashapeOptions = useMemo(() => {
    return Jin10CalendarDatashapeOptions.map((x) => ({
      value: x,
      label: t(`jin10CalendarDatashapeOptions.${x}`),
    }));
  }, [t]);

  const jin10SymbolsTypeOptions = useMemo(() => {
    return Jin10SymbolsTypeOptions.map((x) => ({
      value: x,
      label: t(`jin10SymbolsTypeOptions.${x}`),
    }));
  }, [t]);

  const jin10SymbolsDatatypeOptions = useMemo(() => {
    return Jin10SymbolsDatatypeOptions.map((x) => ({
      value: x,
      label: t(`jin10SymbolsDatatypeOptions.${x}`),
    }));
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
      <Form.Item label={t('type')} name={'type'} initialValue={'flash'}>
        <Select options={jin10TypeOptions}></Select>
      </Form.Item>
      <Form.Item label={t('secretKey')} name={'secret_key'}>
        <Input></Input>
      </Form.Item>
      <Form.Item noStyle dependencies={['type']}>
        {({ getFieldValue }) => {
          const type = getFieldValue('type');
          switch (type) {
            case 'flash':
              return (
                <>
                  <Form.Item label={t('flashType')} name={'flash_type'}>
                    <Select options={jin10FlashTypeOptions}></Select>
                  </Form.Item>
                  <Form.Item label={t('contain')} name={'contain'}>
                    <Input></Input>
                  </Form.Item>
                  <Form.Item label={t('filter')} name={'filter'}>
                    <Input></Input>
                  </Form.Item>
                </>
              );

            case 'calendar':
              return (
                <>
                  <Form.Item label={t('calendarType')} name={'calendar_type'}>
                    <Select options={jin10CalendarTypeOptions}></Select>
                  </Form.Item>
                  <Form.Item
                    label={t('calendarDatashape')}
                    name={'calendar_datashape'}
                  >
                    <Select options={jin10CalendarDatashapeOptions}></Select>
                  </Form.Item>
                </>
              );

            case 'symbols':
              return (
                <>
                  <Form.Item label={t('symbolsType')} name={'symbols_type'}>
                    <Select options={jin10SymbolsTypeOptions}></Select>
                  </Form.Item>
                  <Form.Item
                    label={t('symbolsDatatype')}
                    name={'symbols_datatype'}
                  >
                    <Select options={jin10SymbolsDatatypeOptions}></Select>
                  </Form.Item>
                </>
              );

            case 'news':
              return (
                <>
                  <Form.Item label={t('contain')} name={'contain'}>
                    <Input></Input>
                  </Form.Item>
                  <Form.Item label={t('filter')} name={'filter'}>
                    <Input></Input>
                  </Form.Item>
                </>
              );

            default:
              return <></>;
          }
        }}
      </Form.Item>
    </Form>
  );
};

export default Jin10Form;
