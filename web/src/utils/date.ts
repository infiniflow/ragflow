import dayjs from 'dayjs';

export function formatDate(date: any, format?: string) {
  const thisFormat = format || 'DD/MM/YYYY HH:mm:ss';
  if (!date) {
    return '';
  }
  return dayjs(date).format(thisFormat);
}

export function formatTime(date: any) {
  if (!date) {
    return '';
  }
  return dayjs(date).format('HH:mm:ss');
}

export function today() {
  return formatDate(dayjs());
}

export function lastDay() {
  return formatDate(dayjs().subtract(1, 'days'));
}

export function lastWeek() {
  return formatDate(dayjs().subtract(1, 'weeks'));
}

export function formatPureDate(date: any) {
  if (!date) {
    return '';
  }
  return dayjs(date).format('DD/MM/YYYY');
}

export function formatStandardDate(date: any) {
  if (!date) {
    return '';
  }
  const parsedDate = dayjs(date);
  if (!parsedDate.isValid()) {
    return '';
  }
  return parsedDate.format('YYYY-MM-DD');
}

export function formatSecondsToHumanReadable(seconds: number): string {
  if (isNaN(seconds) || seconds < 0) {
    return '0s';
  }

  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  // const s = toFixed(seconds % 60, 3);
  const s = seconds % 60;
  const formattedSeconds = s === 0 ? '0' : s.toFixed(3).replace(/\.?0+$/, '');
  const parts = [];
  if (h > 0) parts.push(`${h}h `);
  if (m > 0) parts.push(`${m}m `);
  if (s || parts.length === 0) parts.push(`${formattedSeconds}s`);

  return parts.join('');
}
