/*
* Open Chinese Convert
*
* Copyright 2010 BYVoid <byvoid.kcp@gmail.com>
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*/

#ifndef __CONVERTER_H_
#define __CONVERTER_H_

#include "dictionary_set.h"

typedef void * converter_t;

typedef enum
{
    CONVERTER_ERROR_VOID,
    CONVERTER_ERROR_NODICT,
    CONVERTER_ERROR_OUTBUF,
} converter_error;

void converter_assign_dictionary(converter_t t_converter, dictionary_set_t dictionary_set);

converter_t converter_open(void);

void converter_close(converter_t t_converter);

size_t converter_convert(converter_t t_converter, ucs4_t ** inbuf, size_t * inbuf_left,
                         ucs4_t ** outbuf, size_t * outbuf_left);

void converter_set_conversion_mode(converter_t t_converter, opencc_conversion_mode conversion_mode);

converter_error converter_errno(void);

void converter_perror(const char * spec);

#endif /* __CONVERTER_H_ */
