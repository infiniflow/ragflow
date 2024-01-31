import moment from 'moment';

export function today() {
  return formatDate(moment());
}

export function lastDay() {
  return formatDate(moment().subtract(1, 'days'));
}

export function lastWeek() {
  return formatDate(moment().subtract(1, 'weeks'));
}

export function formatDate(date: any) {
  if (!date) {
    return '';
  }
  return moment(date).format('DD/MM/YYYY');
}
