import { Form } from 'antd';
import { useEffect, useState } from 'react';

export const useValidateSubmittable = () => {
  const [form] = Form.useForm();
  const [submittable, setSubmittable] = useState<boolean>(false);

  // Watch all values
  const values = Form.useWatch([], form);

  useEffect(() => {
    form
      .validateFields({ validateOnly: true })
      .then(() => setSubmittable(true))
      .catch(() => setSubmittable(false));
  }, [form, values]);

  return { submittable, form };
};
