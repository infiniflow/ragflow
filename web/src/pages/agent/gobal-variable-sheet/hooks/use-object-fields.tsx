import { BlockButton, Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Segmented } from '@/components/ui/segmented';
import { Editor } from '@monaco-editor/react';
import { t } from 'i18next';
import { Trash2, X } from 'lucide-react';
import { useCallback } from 'react';
import { FieldValues } from 'react-hook-form';
import { z } from 'zod';
import { TypesWithArray } from '../constant';

export const useObjectFields = () => {
  const booleanRender = useCallback(
    (field: FieldValues, className?: string) => {
      const fieldValue = field.value ? true : false;
      return (
        <Segmented
          options={
            [
              { value: true, label: 'True' },
              { value: false, label: 'False' },
            ] as any
          }
          sizeType="sm"
          value={fieldValue}
          onChange={field.onChange}
          className={className}
          itemClassName="justify-center flex-1"
        ></Segmented>
      );
    },
    [],
  );

  const objectRender = useCallback((field: FieldValues) => {
    const fieldValue =
      typeof field.value === 'object'
        ? JSON.stringify(field.value, null, 2)
        : JSON.stringify({}, null, 2);
    console.log('object-render-field', field, fieldValue);
    return (
      <Editor
        height={200}
        defaultLanguage="json"
        theme="vs-dark"
        value={fieldValue}
        onChange={field.onChange}
      />
    );
  }, []);

  const objectValidate = useCallback((value: any) => {
    try {
      if (!JSON.parse(value)) {
        throw new Error(t('knowledgeDetails.formatTypeError'));
      }
      return true;
    } catch (e) {
      throw new Error(t('knowledgeDetails.formatTypeError'));
    }
  }, []);

  const arrayStringRender = useCallback((field: FieldValues, type = 'text') => {
    const values = Array.isArray(field.value)
      ? field.value
      : [type === 'number' ? 0 : ''];
    return (
      <>
        {values?.map((item: any, index: number) => (
          <div key={index} className="flex gap-1 items-center">
            <Input
              type={type}
              value={item}
              onChange={(e) => {
                const newValues = [...values];
                newValues[index] = e.target.value;
                field.onChange(newValues);
              }}
            />
            <Button
              variant={'secondary'}
              onClick={() => {
                const newValues = [...values];
                newValues.splice(index, 1);
                field.onChange(newValues);
              }}
            >
              <Trash2 />
            </Button>
          </div>
        ))}
        <BlockButton
          type="button"
          onClick={() => {
            field.onChange([...field.value, '']);
          }}
        >
          {t('flow.add')}
        </BlockButton>
      </>
    );
  }, []);

  const arrayBooleanRender = useCallback(
    (field: FieldValues) => {
      // const values = field.value || [false];
      const values = Array.isArray(field.value) ? field.value : [false];
      return (
        <div className="flex items-center gap-1 flex-wrap ">
          {values?.map((item: any, index: number) => (
            <div
              key={index}
              className="flex gap-1 items-center bg-bg-card rounded-lg border-[0.5px] border-border-button"
            >
              {booleanRender(
                {
                  value: item,
                  onChange: (value) => {
                    values[index] = !!value;
                    field.onChange(values);
                  },
                },
                'bg-transparent',
              )}
              <Button
                variant={'transparent'}
                className="border-none py-0 px-1"
                onClick={() => {
                  const newValues = [...values];
                  newValues.splice(index, 1);
                  field.onChange(newValues);
                }}
              >
                <X />
              </Button>
            </div>
          ))}
          <BlockButton
            className="w-auto"
            type="button"
            onClick={() => {
              field.onChange([...field.value, false]);
            }}
          >
            {t('flow.add')}
          </BlockButton>
        </div>
      );
    },
    [booleanRender],
  );

  const arrayNumberRender = useCallback(
    (field: FieldValues) => {
      return arrayStringRender(field, 'number');
    },
    [arrayStringRender],
  );

  const arrayValidate = useCallback((value: any, type: string = 'string') => {
    if (!Array.isArray(value) || !value.every((item) => typeof item === type)) {
      throw new Error(t('flow.formatTypeError'));
    }
    return true;
  }, []);

  const arrayStringValidate = useCallback(
    (value: any) => {
      return arrayValidate(value, 'string');
    },
    [arrayValidate],
  );

  const arrayNumberValidate = useCallback(
    (value: any) => {
      return arrayValidate(value, 'number');
    },
    [arrayValidate],
  );

  const arrayBooleanValidate = useCallback(
    (value: any) => {
      return arrayValidate(value, 'boolean');
    },
    [arrayValidate],
  );

  const handleRender = (value: TypesWithArray) => {
    switch (value) {
      case TypesWithArray.Boolean:
        return booleanRender;
      case TypesWithArray.Object:
      case TypesWithArray.ArrayObject:
        return objectRender;
      case TypesWithArray.ArrayString:
        return arrayStringRender;
      case TypesWithArray.ArrayNumber:
        return arrayNumberRender;
      case TypesWithArray.ArrayBoolean:
        return arrayBooleanRender;
      default:
        return undefined;
    }
  };
  const handleCustomValidate = (value: TypesWithArray) => {
    switch (value) {
      case TypesWithArray.Object:
      case TypesWithArray.ArrayObject:
        return objectValidate;
      case TypesWithArray.ArrayString:
        return arrayStringValidate;
      case TypesWithArray.ArrayNumber:
        return arrayNumberValidate;
      case TypesWithArray.ArrayBoolean:
        return arrayBooleanValidate;
      default:
        return undefined;
    }
  };
  const handleCustomSchema = (value: TypesWithArray) => {
    switch (value) {
      case TypesWithArray.ArrayString:
        return z.array(z.string());
      case TypesWithArray.ArrayNumber:
        return z.array(z.number());
      case TypesWithArray.ArrayBoolean:
        return z.array(z.boolean());
      default:
        return undefined;
    }
  };
  return {
    objectRender,
    objectValidate,
    arrayStringRender,
    arrayStringValidate,
    arrayNumberRender,
    booleanRender,
    arrayBooleanRender,
    arrayNumberValidate,
    arrayBooleanValidate,
    handleRender,
    handleCustomValidate,
    handleCustomSchema,
  };
};
