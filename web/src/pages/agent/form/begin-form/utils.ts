import { BeginQuery } from '../../interface';

export function buildBeginInputListFromObject(
  inputs: Record<string, Omit<BeginQuery, 'key'>>,
) {
  return Object.entries(inputs || {}).reduce<BeginQuery[]>(
    (pre, [key, value]) => {
      pre.push({ ...(value || {}), key });

      return pre;
    },
    [],
  );
}

const Ipv4Segment = '(25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)';
const Ipv4Pattern = new RegExp(`^(${Ipv4Segment}\\.){3}${Ipv4Segment}$`);
const Ipv6HextetPattern = /^[0-9a-fA-F]{1,4}$/;
const DigitsPattern = /^\d+$/;

function isValidIpv6(value: string): boolean {
  let rest = value;

  // An embedded IPv4 tail (e.g. ::ffff:192.168.0.1) counts as two hextets.
  const lastColonIndex = rest.lastIndexOf(':');
  const ipv4Tail = rest.slice(lastColonIndex + 1);
  if (lastColonIndex !== -1 && ipv4Tail.includes('.')) {
    if (!Ipv4Pattern.test(ipv4Tail)) {
      return false;
    }
    rest = `${rest.slice(0, lastColonIndex)}:0:0`;
  } else if (rest.includes('.')) {
    return false;
  }

  const halves = rest.split('::');
  if (halves.length > 2) {
    return false;
  }

  const head = halves[0] ? halves[0].split(':') : [];
  const tail = halves.length === 2 && halves[1] ? halves[1].split(':') : [];

  if (![...head, ...tail].every((hextet) => Ipv6HextetPattern.test(hextet))) {
    return false;
  }

  // '::' must compress at least one hextet; without it exactly 8 are required.
  return halves.length === 2
    ? head.length + tail.length < 8
    : head.length === 8;
}

// Accepts a bare IPv4/IPv6 address or one with a CIDR prefix length,
// matching what the webhook backend understands (e.g. 10.0.0.5, 10.0.0.0/8).
export function isValidIpOrCidr(value: string): boolean {
  const parts = value.split('/');
  if (parts.length > 2) {
    return false;
  }

  const [address, prefixLength] = parts;
  const isV4 = Ipv4Pattern.test(address);
  if (!isV4 && !isValidIpv6(address)) {
    return false;
  }

  if (prefixLength === undefined) {
    return true;
  }
  if (!DigitsPattern.test(prefixLength)) {
    return false;
  }

  const bits = Number(prefixLength);
  return isV4 ? bits <= 32 : bits <= 128;
}
