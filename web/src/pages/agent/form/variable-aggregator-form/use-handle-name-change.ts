import message from '@/components/ui/message';
import { trim } from 'lodash';
import { ChangeEvent, useCallback, useEffect, useState } from 'react';
import { useFormContext } from 'react-hook-form';
import { VariableAggregatorFormSchemaType } from './schema';

export const useHandleNameChange = (previousName: string) => {
  const [name, setName] = useState<string>('');
  const form = useFormContext<VariableAggregatorFormSchemaType>();

  const handleNameBlur = useCallback(() => {
    const names = form.getValues('groups');
    const existsSameName = names.some((x) => x.group_name === name);
    if (trim(name) === '' || existsSameName) {
      if (existsSameName && previousName !== name) {
        message.error('The name cannot be repeated');
      }
      setName(previousName);
      return previousName;
    }
    return name;
  }, [form, name, previousName]);

  const handleNameChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    const filteredValue = value.replace(/[^a-zA-Z0-9_]/g, '');
    setName(filteredValue);
  }, []);

  useEffect(() => {
    setName(previousName);
  }, [previousName]);

  return {
    name,
    handleNameBlur,
    handleNameChange,
  };
};
