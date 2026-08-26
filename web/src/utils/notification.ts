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

import { ExternalToast, toast } from 'sonner';

const defaultConfig: ExternalToast = { duration: 4000, position: 'top-right' };

type NotificationOptions = {
  message: string;
  description?: string;
  duration?: number;
};

const notification = {
  success: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.success(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
  error: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.error(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
  warning: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.warning(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
  info: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.info(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
};

export default notification;
