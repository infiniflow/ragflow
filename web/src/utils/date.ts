/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import dayjs, { type ConfigType } from 'dayjs';
import utc from 'dayjs/plugin/utc';

// `parseDate` calls `.utc()` for strings that already carry a timezone
// suffix; dayjs only exposes that method once the plugin is registered.
dayjs.extend(utc);

const hasTimezoneSuffix = (value: string) =>
  /(?:GMT|Z|[+-]\d{2}:?\d{2})$/.test(value.trim());

const parseDate = (date: ConfigType) => {
  if (typeof date === 'string' && hasTimezoneSuffix(date)) {
    return dayjs(date).utc();
  }
  return dayjs(date);
};

export function formatDate(date: ConfigType, format?: string) {
  const thisFormat = format || 'DD/MM/YYYY HH:mm:ss';
  if (!date) {
    return '';
  }
  return parseDate(date).format(thisFormat);
}

export function formatTime(date: ConfigType) {
  if (!date) {
    return '';
  }
  return parseDate(date).format('HH:mm:ss');
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

export function formatPureDate(date: ConfigType) {
  if (!date) {
    return '';
  }
  return parseDate(date).format('DD/MM/YYYY');
}

export function formatStandardDate(date: ConfigType) {
  if (!date) {
    return '';
  }
  const parsedDate = parseDate(date);
  if (!parsedDate.isValid()) {
    return '';
  }
  return parsedDate.format('YYYY-MM-DD');
}

export function formatIsoDateTime(timestamp_ms: number): string {
  if (!timestamp_ms) {
    return '';
  }
  return dayjs(timestamp_ms).format('YYYY-MM-DDTHH:mm:ssZ');
}

export function formatDateToLocal(date: ConfigType) {
  if (!date) {
    return '';
  }
  return dayjs(date).format('DD/MM/YYYY HH:mm:ss');
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
  if (h > 0) parts.push(`${h}h`);
  if (m > 0) parts.push(`${m}m`);
  if (s || parts.length === 0) parts.push(`${formattedSeconds}s`);

  return parts.join(' ');
}
