import { convertTheKeysOfTheObjectToSnake } from '../common-util';
test('keeps array request payloads as arrays', () => {
  const body = [{ itemId: 'one' }, { itemId: 'two' }];
  expect(convertTheKeysOfTheObjectToSnake(body)).toBe(body);
});
