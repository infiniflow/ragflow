import { convertTheKeysOfTheObjectToSnake } from '../common-util';
test('preserves Date scalar request values', () => {
  const date = new Date('2026-10-10T00:00:00Z');
  expect(convertTheKeysOfTheObjectToSnake(date)).toBe(date);
  expect(convertTheKeysOfTheObjectToSnake({ modelName: 'x' })).toEqual({
    model_name: 'x',
  });
});

test('normalizes objects with a null prototype', () => {
  const body = Object.assign(Object.create(null), { modelName: 'x' });
  expect(convertTheKeysOfTheObjectToSnake(body)).toEqual({ model_name: 'x' });
});
test('normalizes a plain object from another window', () => {
  const frame = document.createElement('iframe');
  document.body.appendChild(frame);
  const body = new (
    frame.contentWindow as unknown as { Object: ObjectConstructor }
  ).Object();
  Object.assign(body, { modelName: 'x' });
  expect(convertTheKeysOfTheObjectToSnake(body)).toEqual({ model_name: 'x' });
  frame.remove();
});
