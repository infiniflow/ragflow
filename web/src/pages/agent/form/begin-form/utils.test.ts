import { isValidIpOrCidr } from './utils';

describe('isValidIpOrCidr', () => {
  it.each([
    '10.0.0.5',
    '192.168.1.1',
    '0.0.0.0',
    '255.255.255.255',
    '10.0.0.0/8',
    '10.0.0.0/0',
    '10.0.0.0/32',
  ])('accepts IPv4 %s', (value) => {
    expect(isValidIpOrCidr(value)).toBe(true);
  });

  it.each([
    '::1',
    '::',
    '2001:db8::1',
    '2001:0db8:85a3:0000:0000:8a2e:0370:7334',
    '1:2:3:4:5:6:7:8',
    '::ffff:192.168.0.1',
    '2001:db8::/32',
    '::1/128',
  ])('accepts IPv6 %s', (value) => {
    expect(isValidIpOrCidr(value)).toBe(true);
  });

  it.each([
    '',
    'abc',
    '256.1.1.1',
    '1.2.3',
    '1.2.3.4.5',
    '01.2.3.4',
    '10.0.0.0/33',
    '10.0.0.0/',
    '10.0.0.0/abc',
    '10.0.0.0/8/16',
    '1:2:3:4:5:6:7:8:9',
    '2001::db8::1',
    '12345::',
    ':1::',
    '::1/129',
  ])('rejects %s', (value) => {
    expect(isValidIpOrCidr(value)).toBe(false);
  });
});
