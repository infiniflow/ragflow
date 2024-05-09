import dayjs from 'dayjs';

export function today() {
  return formatDate(dayjs());
}

export function lastDay() {
  return formatDate(dayjs().subtract(1, 'days'));
}

export function lastWeek() {
  return formatDate(dayjs().subtract(1, 'weeks'));
}

export function formatDate(date: any) {
  if (!date) {
    return '';
  }
  return dayjs(date).format('DD/MM/YYYY HH:mm:ss');
}
